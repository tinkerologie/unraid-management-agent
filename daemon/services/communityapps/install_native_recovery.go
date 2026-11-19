package communityapps

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"syscall"
)

const maxStartedTemplateRead = 1 << 20

func openPublishedNativeTemplate(dir *os.Root, name string) (*os.File, error) {
	return dir.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

func observePublishedNativeTemplate(
	stat func(string) (fs.FileInfo, error),
	open func(string) (fs.File, error),
	openDirectory func() (nativeRestoreAnchor, error),
	filename, installedName string,
) (startObservation, error) {
	info, err := stat(filename)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return startTemplateMissing, nil
	case err != nil:
		return startUnobserved, ErrNativeTemplate
	case !info.Mode().IsRegular():
		return startTemplateMissing, nil
	}

	file, err := open(filename)
	if err != nil {
		return startUnobserved, ErrNativeTemplate
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return startUnobserved, ErrNativeTemplate
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStartedTemplateRead+1))
	if err != nil {
		return startUnobserved, ErrNativeTemplate
	}
	var template catalogLinkTemplate
	if len(data) > maxStartedTemplateRead || decodeCatalogLinkTemplate(bytes.NewReader(data), &template) != nil ||
		!template.matchesName(installedName) {
		return startTemplateMissing, nil
	}
	// A previous restore may have renamed the template and then lost its
	// directory sync before committing the receipt. Visibility is not proof
	// of durability, even on a later recovery pass.
	directory, err := openDirectory()
	if err != nil {
		return startUnobserved, ErrNativeTemplate
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return startUnobserved, ErrNativeTemplate
	}
	return startVerified, nil
}

// restoreRetainedNativeTemplate recovers an owned publication moved aside by
// an earlier cleanup pass. A replacement or started container may still use it.
func (s *Service) restoreRetainedNativeTemplate(receipt installReceipt) CleanupIssue {
	quarantine := nativePrivatePrefix(receipt) + ".published-cleanup"
	issue := CleanupIssue{Resource: "template", Code: "restore_template"}
	if receipt.Version != 1 {
		issue.TemplateFile = quarantine
	}
	dir, err := s.openNativeTemplates()
	if errors.Is(err, fs.ErrNotExist) {
		return CleanupIssue{}
	}
	if err != nil {
		return issue
	}
	defer func() { _ = dir.Close() }()

	return restorePublishedQuarantine(dir, receipt, issue, quarantine, s.finalTemplateName(receipt), func() (nativeRestoreAnchor, error) {
		return dir.Open(".")
	})
}

type nativeRestoreAnchor interface {
	Fd() uintptr
	Sync() error
	Close() error
}

func restorePublishedQuarantine(
	dir *os.Root, receipt installReceipt, issue CleanupIssue, quarantine, target string, openAnchor func() (nativeRestoreAnchor, error),
) CleanupIssue {
	if _, err := dir.Lstat(quarantine); errors.Is(err, fs.ErrNotExist) {
		return CleanupIssue{}
	}
	owned, err := nativeFileOwnership(dir, quarantine, receipt, false)
	if err != nil || !owned {
		return issue
	}
	issue.OwnershipVerified = true
	anchor, err := openAnchor()
	if err != nil {
		return issue
	}
	defer func() { _ = anchor.Close() }()
	if err := renameExclusiveAt(int(anchor.Fd()), quarantine, target); err != nil {
		return issue
	}
	if err := anchor.Sync(); err != nil {
		issue.TemplateFile = target
		return issue
	}
	return CleanupIssue{}
}
