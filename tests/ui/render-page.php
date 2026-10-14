<?php
// Renders the plugin settings page to static HTML for the Playwright UI tests.
// Unraid serves .page files through its webGUI: the header up to the first
// "---" line is metadata and the rest is PHP/HTML. This keeps the PHP part and
// wraps it in a minimal document with jQuery, which the webGUI provides.
//
// Usage: php render-page.php <page file> <output html>

$page = $argv[1] ?? '';
$out = $argv[2] ?? '';
if ($page === '' || $out === '') {
    fwrite(STDERR, "usage: php render-page.php <page file> <output html>\n");
    exit(2);
}
$source = file_get_contents($page);
$parts = ($source === false) ? false : preg_split('/^---\s*$/m', $source, 2);
if ($parts === false || count($parts) !== 2) {
    fwrite(STDERR, "cannot read the page body from $page\n");
    exit(1);
}
// The webGUI's request context; any PHP warning fails the render.
$_SERVER['SERVER_ADDR'] = '192.0.2.10';
$_SERVER['HTTP_HOST'] = $_SERVER['HTTP_HOST'] ?? 'tower.local';
set_error_handler(function ($severity, $message, $file, $line) {
    throw new ErrorException($message, 0, $severity, $file, $line);
});

$tmp = tempnam(sys_get_temp_dir(), 'uma-page-');
file_put_contents($tmp, $parts[1]);
ob_start();
include $tmp;
$body = ob_get_clean();
unlink($tmp);

$html = "<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>UMA settings</title>"
    . "<script src=\"/jquery.js\"></script></head><body>\n" . $body . "\n</body></html>\n";
if (file_put_contents($out, $html) === false) {
    fwrite(STDERR, "cannot write $out\n");
    exit(1);
}
