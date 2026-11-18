// UI tests for the collector intervals added to the plugin settings page (#276).
// The page is rendered by global-setup.js without a config file, so every
// select shows its fallback default; every network call is mocked here.
const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');

const BASE = 'http://uma.test';
const RENDER = path.join(__dirname, '..', '.render');
const JQUERY = path.join(__dirname, '..', 'node_modules', 'jquery', 'dist', 'jquery.min.js');

// Select name, row label, the daemon default (main.go), the value set in the
// test config.cfg (global-setup.js) and an option list value.
const INTERVALS = [
  { name: 'INTERVAL_DISK', label: 'Disk Health', def: '300', configured: '30', fastest: null },
  { name: 'INTERVAL_FAN_CONTROL', label: 'Fan Control', def: '60', configured: '30', fastest: '5' },
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
  await page.locator('select[name="INTERVAL_FAN_CONTROL"]').selectOption('5');
  const mover = page.locator('select[name="INTERVAL_MOVER"]');
  await mover.selectOption('0');
  await expect(mover).toHaveClass(/\bis-disabled\b/);
  await expect(mover).toHaveCSS('font-weight', '600');
  const fields = await form.evaluate((f) => Object.fromEntries(new FormData(f)));
  expect(fields.INTERVAL_FAN_CONTROL).toBe('5');
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

test('Apply preserves unlisted intervals even while Dynamic collection hides them', async ({ page }) => {
  await openPage(page, 'page-dynamic.html');
  await expect(page.locator('#uma-manual-intervals')).toBeHidden();
  const form = page.locator('#agentConfigForm');
  const fields = await form.evaluate((f) => Object.fromEntries(new FormData(f)));
  expect(fields.INTERVAL_DISK).toBe('30');
  expect(fields.INTERVAL_ARRAY).toBe('37');

  // The WebGUI enables Apply and supplies this iframe around a plugin page.
  // Capture the browser's actual form submission without writing host settings.
  await page.evaluate(() => {
    document.body.insertAdjacentHTML('beforeend', '<iframe name="progressFrame" hidden></iframe>');
    document.querySelector('input[name="#apply"]').disabled = false;
  });
  await page.route('**/update.php', (route) => route.fulfill({ body: 'Settings saved' }));
  await page.locator('#uma_port_input').fill('9043');
  const request = page.waitForRequest((posted) => new URL(posted.url()).pathname === '/update.php');
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  const posted = await request;
  expect(posted.method()).toBe('POST');
  const payload = new URLSearchParams(posted.postData());
  expect(payload.get('INTERVAL_DISK')).toBe('30');
  expect(payload.get('INTERVAL_ARRAY')).toBe('37');
  expect(payload.get('COLLECTION_MODE')).toBe('dynamic');
  expect(payload.get('PORT')).toBe('9043');

  const saved = fs.mkdtempSync(path.join(RENDER, 'applied-'));
  try {
    const config = path.join(saved, 'config.cfg');
    const rendered = path.join(saved, 'page.html');
    const keys = ['COLLECTION_MODE', 'INTERVAL_DISK', 'INTERVAL_ARRAY', 'PORT'];
    fs.writeFileSync(config, keys.map((key) => `${key}=${JSON.stringify(payload.get(key))}\n`).join(''));
    execFileSync('php', [
      path.join(__dirname, '..', 'render-page.php'),
      path.join(__dirname, '..', '..', '..', 'meta', 'plugin', 'unraid-management-agent.page'),
      rendered,
      config,
    ]);
    await page.route(`${BASE}/`, (route) => route.fulfill({ path: rendered, contentType: 'text/html' }));
    await page.reload();
    await expect(page.locator('#uma_port_input')).toHaveValue('9043');
    await expect(page.locator('#uma-manual-intervals')).toBeHidden();
    await expect(page.locator('select[name="INTERVAL_DISK"]')).toHaveValue('30');
    await expect(page.locator('select[name="INTERVAL_ARRAY"]')).toHaveValue('37');
  } finally {
    fs.rmSync(saved, { recursive: true });
  }
});
