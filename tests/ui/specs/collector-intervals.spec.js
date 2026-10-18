// UI tests for the collector intervals added to the plugin settings page (#276).
// The page is rendered by global-setup.js without a config file, so every
// select shows its fallback default; every network call is mocked here.
const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const BASE = 'http://uma.test';
const RENDER = path.join(__dirname, '..', '.render');
const JQUERY = path.join(__dirname, '..', 'node_modules', 'jquery', 'dist', 'jquery.min.js');

// Select name, row label, the daemon default (main.go), the value set in the
// test config.cfg (global-setup.js) and an option list value.
const INTERVALS = [
  { name: 'INTERVAL_FAN_CONTROL', label: 'Fan Control', def: '5', configured: '60', fastest: '5' },
  { name: 'INTERVAL_TUNING', label: 'Tuning Parameters', def: '120', configured: '300', fastest: '10' },
  { name: 'INTERVAL_DOCKER_UPDATE', label: 'Docker Update Checks', def: '21600', configured: '86400', fastest: '10' },
  { name: 'INTERVAL_DOCKER_NETWORKS', label: 'Docker Networks', def: '60', configured: '0', fastest: '10' },
  { name: 'INTERVAL_PLUGIN_UPDATE', label: 'Plugin Update Checks', def: '3600', configured: '43200', fastest: '10' },
  { name: 'INTERVAL_OS_UPDATE', label: 'OS Update Checks', def: '86400', configured: '3600', fastest: null },
  { name: 'INTERVAL_MOVER', label: 'Mover Status', def: '30', configured: '120', fastest: '10' },
];

const json = (body, status = 200) => ({ status, contentType: 'application/json', body: JSON.stringify(body) });

async function openPage(page, html = 'page.html') {
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/') {
      return route.fulfill({ contentType: 'text/html', body: fs.readFileSync(path.join(RENDER, html), 'utf8') });
    }
    if (url.pathname === '/jquery.js') {
      return route.fulfill({ contentType: 'application/javascript', body: fs.readFileSync(JQUERY, 'utf8') });
    }
    if (url.pathname.endsWith('/include/diagnostics.php')) {
      return route.fulfill(json({ error: 'Could not reach the agent' }, 502));
    }
    if (url.pathname.endsWith('/include/exec.php')) {
      return route.fulfill(json({ running: true }));
    }
    return route.fulfill({ status: 404, body: '' });
  });
  await page.goto(`${BASE}/`);
}

test('new interval selects show the daemon defaults and can be disabled', async ({ page }) => {
  await openPage(page);
  for (const { name, label, def, fastest } of INTERVALS) {
    const select = page.locator(`select[name="${name}"]`);
    await expect(select, name).toHaveCount(1);
    await expect(select, name).toHaveValue(def);
    await expect(page.locator('.uma-interval-row', { has: select }), name).toContainText(label);
    await expect(select.locator('option[value="0"]'), name).toHaveText('Disabled');
    await expect(select.locator('option[value="86400"]'), name).toHaveCount(1);
    if (fastest) await expect(select.locator(`option[value="${fastest}"]`), name).toHaveCount(1);
  }
});

test('fan control offers 5 seconds and OS updates use the slow list', async ({ page }) => {
  await openPage(page);
  await expect(page.locator('select[name="INTERVAL_FAN_CONTROL"] option[value="5"]')).toHaveCount(1);
  await expect(page.locator('select[name="INTERVAL_DOCKER_NETWORKS"] option[value="5"]')).toHaveCount(0);
  await expect(page.locator('select[name="INTERVAL_OS_UPDATE"] option[value="10"]')).toHaveCount(0);
});

test('new interval values are submitted with the form', async ({ page }) => {
  await openPage(page);
  const form = page.locator('select[name="INTERVAL_FAN_CONTROL"]').locator('xpath=ancestor::form');
  await page.locator('select[name="INTERVAL_FAN_CONTROL"]').selectOption('60');
  await page.locator('select[name="INTERVAL_MOVER"]').selectOption('0');
  const fields = await form.evaluate((f) => Object.fromEntries(new FormData(f)));
  expect(fields.INTERVAL_FAN_CONTROL).toBe('60');
  expect(fields.INTERVAL_MOVER).toBe('0');
  for (const { name, def } of INTERVALS.filter((i) => !['INTERVAL_FAN_CONTROL', 'INTERVAL_MOVER'].includes(i.name))) {
    expect(fields[name], name).toBe(def);
  }
});

test('new interval selects show the values saved in config.cfg', async ({ page }) => {
  await openPage(page, 'page-configured.html');
  for (const { name, configured } of INTERVALS) {
    await expect(page.locator(`select[name="${name}"]`), name).toHaveValue(configured);
  }
});
