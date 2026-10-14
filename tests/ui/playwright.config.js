// Playwright configuration for the settings-page UI tests (#261).
// globalSetup renders the .page file with the PHP CLI; the tests serve that
// HTML and mock the plugin's PHP endpoints with page.route, so no web server,
// Unraid or agent is needed.
const { defineConfig, devices } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './specs',
  globalSetup: require.resolve('./global-setup.js'),
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    headless: true,
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
