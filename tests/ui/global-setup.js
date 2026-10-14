// Renders the plugin settings page to .render/page.html before the tests run.
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

module.exports = async () => {
  const out = path.join(__dirname, '.render');
  fs.mkdirSync(out, { recursive: true });
  execFileSync(
    'php',
    [
      path.join(__dirname, 'render-page.php'),
      path.join(__dirname, '..', '..', 'meta', 'plugin', 'unraid-management-agent.page'),
      path.join(out, 'page.html'),
    ],
    { stdio: 'inherit' },
  );
};
