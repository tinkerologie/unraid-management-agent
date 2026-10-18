<?php
// Renders the plugin settings page to static HTML for the Playwright UI tests.
// Unraid serves .page files through its webGUI: the header up to the first
// "---" line is metadata and the rest is PHP/HTML. This keeps the PHP part and
// wraps it in a minimal document with jQuery, which the webGUI provides.
//
// Usage: php render-page.php <page file> <output html> [config.cfg]

$page = $argv[1] ?? '';
$out = $argv[2] ?? '';
$configFile = $argv[3] ?? '/nonexistent/uma-ui-test/config.cfg';
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

// Render with the given config, or none, never the host's saved settings, so
// the tests see known values even on an Unraid server with a config.cfg.
$body = preg_replace('/^\$config_file = .*;$/m', '$config_file = ' . var_export($configFile, true) . ';', $parts[1], 1, $count);
if ($count !== 1) {
    fwrite(STDERR, "cannot find the \$config_file assignment in $page\n");
    exit(1);
}

$tmp = tempnam(sys_get_temp_dir(), 'uma-page-');
file_put_contents($tmp, $body);
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
