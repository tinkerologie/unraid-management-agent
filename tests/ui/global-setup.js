// Renders the plugin settings page before the tests run: .render/page.html
// without a config file (fallback defaults) and .render/page-configured.html
// with .render/config.cfg, which sets non-default collector intervals.
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

module.exports = async () => {
  const out = path.join(__dirname, '.render');
  fs.mkdirSync(out, { recursive: true });
  const page = path.join(__dirname, '..', '..', 'meta', 'plugin', 'unraid-management-agent.page');
  const render = (html, ...config) =>
    execFileSync('php', [path.join(__dirname, 'render-page.php'), page, path.join(out, html), ...config], {
      stdio: 'inherit',
    });
  render('page.html');

  const config = path.join(out, 'config.cfg');
  fs.writeFileSync(
    config,
    [
      'INTERVAL_FAN_CONTROL="60"',
      'INTERVAL_TUNING="300"',
      'INTERVAL_DOCKER_UPDATE="86400"',
      'INTERVAL_DOCKER_NETWORKS="0"',
      'INTERVAL_PLUGIN_UPDATE="43200"',
      'INTERVAL_OS_UPDATE="3600"',
      'INTERVAL_MOVER="120"',
      '',
    ].join('\n'),
  );
  render('page-configured.html', config);
};
