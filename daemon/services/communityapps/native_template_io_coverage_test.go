package communityapps

import (
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestNativeTemplatePublicationKeepsStageAndFinalPathsConsistentOnIOErrors(t *testing.T) {
	for _, stage := range []string{"write", "file sync", "file close", "staging directory sync", "name collision", "rename", "published directory sync"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			state := &publishFileFixture{failure: stage}
			syncCount := 0
			const staged = "install.tmp"
			const final = "my-existing.xml"
			publisher := templatePublisher{
				openStaging: func(name string) (nativeTemplateWriter, error) {
					var err error
					state.file, err = openNativeStaging(name)
					return state, err
				},
				syncDir: func(dirname string) error {
					syncCount++
					if stage == "staging directory sync" && syncCount == 1 || stage == "published directory sync" && syncCount == 2 {
						return syscall.EIO
					}
					return syncDirectory(dirname)
				},
				move: func(source, target string) error {
					switch stage {
					case "name collision":
						if err := os.WriteFile(target, []byte("operator's file"), 0o600); err != nil {
							return err
						}
					case "rename":
						return syscall.EIO
					}
					return renameExclusive(source, target)
				},
			}
			want := ErrNativeTemplate
			if stage == "name collision" {
				want = ErrNameConflict
			}
			err := publisher.publish(dir, staged, final, []byte("candidate"))
			if !errors.Is(err, want) || state.closeCount != 1 {
				t.Fatalf("publish returned %v with %d closes, want %v and one close", err, state.closeCount, want)
			}
			target := filepath.Join(dir, final)
			switch stage {
			case "name collision":
				assertNativeFile(t, target, "operator's file")
			case "published directory sync":
				assertNativeFile(t, target, "candidate")
			default:
				if _, err := os.Stat(target); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("pre-publication error left visible template: %v", err)
				}
			}
		})
	}
}

type publishFileFixture struct {
	file       nativeTemplateWriter
	failure    string
	closeCount int
}

func (f *publishFileFixture) Write(data []byte) (int, error) {
	if f.failure == "write" {
		return 0, syscall.EIO
	}
	return f.file.Write(data)
}

func (f *publishFileFixture) Sync() error {
	if f.failure == "file sync" {
		return syscall.EIO
	}
	return f.file.Sync()
}

func (f *publishFileFixture) Close() error {
	f.closeCount++
	err := f.file.Close()
	if f.failure == "file close" {
		return syscall.EIO
	}
	return err
}

func TestNativeTemplateDirectoryRejectsReplacementsBetweenStatAndOpen(t *testing.T) {
	for _, failOpen := range []bool{false, true} {
		root := t.TempDir()
		communityappstest.WriteFile(t, root, userTemplatesDir+"/marker", "native data")
		dir, err := openNativeDirectory(root, func(parent *os.Root, name string) (*os.Root, error) {
			if name == "templates-user" {
				if failOpen {
					return nil, fs.ErrPermission
				}
				if err := parent.Rename(name, name+".previous"); err != nil {
					t.Fatal(err)
				}
				if err := parent.Mkdir(name, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			return parent.OpenRoot(name)
		})
		if dir != nil {
			_ = dir.Close()
		}
		if !errors.Is(err, ErrNativeTemplate) || dir != nil {
			t.Fatalf("replaced or unavailable native directory returned %v, %v", dir, err)
		}
	}
}

func TestNativeTemplateOwnershipFailsClosedOnDescriptorAndReadErrors(t *testing.T) {
	root := t.TempDir()
	const filename = "my-sample.xml"
	communityappstest.WriteFile(t, root, filename, "original")
	communityappstest.WriteFile(t, root, "different.xml", "replacement")
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	original, err := dir.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := dir.Stat("different.xml")
	if err != nil {
		t.Fatal(err)
	}
	receipt := templateCoverageReceipt(2)
	payload, err := prepareNativeTemplate(installSettings{}, &receipt, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(string(payload), "\n")
	marker, _, _ := strings.Cut(rest, "\n")

	for _, tc := range []struct {
		name        string
		statError   error
		actual      fs.FileInfo
		partialRead string
	}{
		{name: "unreadable directory", statError: syscall.EIO, actual: original},
		{name: "descriptor points at a replacement", actual: replacement},
		{name: "header read", actual: original},
		{name: "marker read", actual: original, partialRead: xml.Header},
		{name: "XML body read", actual: original, partialRead: xml.Header + marker + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owned, err := readNativeOwnership(
				func(string) (fs.FileInfo, error) { return original, tc.statError },
				func(string) (nativeTemplateReader, error) {
					contents := io.MultiReader(strings.NewReader(tc.partialRead), failingTemplateRead{})
					return ownershipFaultFile{Reader: contents, info: tc.actual}, nil
				},
				filename, receipt, false,
			)
			if owned || !errors.Is(err, ErrNativeTemplate) {
				t.Fatalf("unreliable proof returned %t, %v", owned, err)
			}
		})
	}
}

type failingTemplateRead struct{}

func (failingTemplateRead) Read([]byte) (int, error) { return 0, syscall.EIO }

type ownershipFaultFile struct {
	io.Reader
	info fs.FileInfo
}

func (f ownershipFaultFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (ownershipFaultFile) Close() error                 { return nil }

func TestNativeTemplateCleanupDistinguishesLostProofAndDurabilityErrors(t *testing.T) {
	for _, failure := range []string{"anchor open", "quarantine stat", "rename disappears", "rename fails", "directory sync"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			service := NewService(nil, ServiceOptions{HostRoot: root})
			receipt := templateCoverageReceipt(2)
			payload, err := prepareNativeTemplate(installSettings{}, &receipt, 0)
			if err != nil {
				t.Fatal(err)
			}
			target := service.finalTemplateName(receipt)
			quarantine := nativePrivatePrefix(receipt) + ".published-cleanup"
			targetPath := filepath.Join(root, userTemplatesDir, target)
			communityappstest.WriteFile(t, root, filepath.Join(userTemplatesDir, target), string(payload))
			io := templateCleanupIO{
				openAnchor: func(dir *os.Root) (templateDirectoryAnchor, error) {
					if failure == "anchor open" {
						return nil, fs.ErrPermission
					}
					fd, err := dir.Open(".")
					if err != nil {
						return nil, err
					}
					return syncFaultAnchor{fd, failure == "directory sync"}, nil
				},
				statCleanup: func(dir *os.Root, name string) (fs.FileInfo, error) {
					if failure == "quarantine stat" {
						return nil, syscall.EIO
					}
					return dir.Lstat(name)
				},
				move: func(fd int, from, to string) error {
					switch failure {
					case "rename disappears":
						if err := os.Remove(targetPath); err != nil {
							t.Fatal(err)
						}
					case "rename fails":
						return syscall.EIO
					}
					return renameExclusiveAt(fd, from, to)
				},
			}
			issue := service.removeNativeFile(receipt, false, io)
			switch failure {
			case "rename disappears":
				if issue.Code != "" {
					t.Fatalf("concurrent removal should complete idempotently: %+v", issue)
				}
			case "directory sync", "quarantine stat":
				if issue.TemplateFile != quarantine || issue.OwnershipVerified {
					t.Fatalf("uncertain cleanup did not identify the quarantine path: %+v", issue)
				}
			default:
				if issue.TemplateFile != target || issue.OwnershipVerified != (failure == "rename fails") {
					t.Fatalf("incomplete cleanup reported incorrect proof: %+v", issue)
				}
				assertNativeFile(t, targetPath, string(payload))
			}
		})
	}
}

type syncFaultAnchor struct {
	*os.File
	failSync bool
}

func (a syncFaultAnchor) Sync() error {
	if a.failSync {
		return syscall.EIO
	}
	return a.File.Sync()
}
