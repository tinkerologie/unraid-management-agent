// UI tests for the Dynamic collection toggle on the plugin settings page (#280).
// global-setup.js renders page.html without a config (manual intervals) and
// page-dynamic.html with COLLECTION_MODE="dynamic" and saved non-default
// intervals; every network call is mocked here.
const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const BASE = 'http://uma.test';
const RENDER = path.join(__dirname, '..', '.render');
const JQUERY = path.join(__dirname, '..', 'node_modules', 'jquery', 'dist', 'jquery.min.js');

// Saved intervals in the page-dynamic.html config (global-setup.js).
const SAVED = { INTERVAL_SYSTEM: '30', INTERVAL_DOCKER: '10', INTERVAL_MOVER: '120' };

const CAPABILITIES = {
  checked_at: '2026-10-18T09:00:00Z',
  items: [
    { name: 'hardware.ups', state: 'present', detail: 'apcupsd' },
    { name: 'hardware.zfs', state: 'absent', detail: 'no pools' },
  ],
};

const json = (body, status = 200) => ({ status, contentType: 'application/json', body: JSON.stringify(body) });

// Opens a rendered page and returns the posted diagnostics requests.
async function openPage(page, html) {
  const requests = [];
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/') {
      return route.fulfill({ contentType: 'text/html', body: fs.readFileSync(path.join(RENDER, html), 'utf8') });
    }
    if (url.pathname === '/jquery.js') {
      return route.fulfill({ contentType: 'application/javascript', body: fs.readFileSync(JQUERY, 'utf8') });
    }
    if (url.pathname.endsWith('/include/diagnostics.php')) {
      const form = Object.fromEntries(new URLSearchParams(route.request().postData() || ''));
      requests.push(form);
      if (form.action === 'capabilities') return route.fulfill(json(CAPABILITIES));
      return route.fulfill(json({ error: 'unexpected' }, 400));
    }
    if (url.pathname.endsWith('/include/exec.php')) {
      return route.fulfill(json({ running: true }));
    }
    return route.fulfill({ status: 404, body: '' });
  });
  await page.goto(`${BASE}/`);
  return requests;
}

const toggle = (page) => page.locator('label[for="collection_mode"]');
const manual = (page) => page.locator('#uma-manual-intervals');
const formFields = (page) =>
  page.locator('#collection_mode').evaluate((el) => Object.fromEntries(new FormData(el.closest('form'))));

// Everything under the toggle that Dynamic mode hides.
const MANUAL_ONLY = [
  'text=How often data is collected',
  'label[for="show_all_collectors"]',
  '#uma-hw-recheck-btn',
  '#uma-hw-status',
  'select[name="INTERVAL_SYSTEM"]',
  '.uma-note:has-text("Power Tip")',
];

test('Dynamic collection hides every interval setting under the toggle', async ({ page }) => {
  await openPage(page, 'page.html');
  await expect(page.locator('#collection_mode')).not.toBeChecked();
  await expect(page.locator('#collection_mode_val')).toHaveValue('manual');
  for (const sel of MANUAL_ONLY) await expect(page.locator(sel).first(), sel).toBeVisible();

  await toggle(page).click();
  await expect(page.locator('#collection_mode')).toBeChecked();
  await expect(page.locator('#collection_mode_val')).toHaveValue('dynamic');
  await expect(toggle(page)).toBeVisible();
  for (const sel of MANUAL_ONLY) await expect(page.locator(sel).first(), sel).toBeHidden();

  await toggle(page).click();
  await expect(page.locator('#collection_mode_val')).toHaveValue('manual');
  for (const sel of MANUAL_ONLY) await expect(page.locator(sel).first(), sel).toBeVisible();
});

test('a dynamic config renders with the interval settings hidden', async ({ page }) => {
  await openPage(page, 'page-dynamic.html');
  await expect(page.locator('#collection_mode')).toBeChecked();
  await expect(manual(page)).toBeHidden();
  await expect(toggle(page)).toBeVisible();
});

test('hidden intervals are still submitted and shown again after switching back', async ({ page }) => {
  await openPage(page, 'page-dynamic.html');
  await expect(manual(page)).toBeHidden();

  const fields = await formFields(page);
  expect(fields.COLLECTION_MODE).toBe('dynamic');
  expect(fields.COLLECTION_MODE_CB).toBe('on');
  for (const [name, value] of Object.entries(SAVED)) expect(fields[name], name).toBe(value);
  const selects = await page.locator('.uma-interval-select').evaluateAll((els) => els.map((s) => [s.name, s.disabled]));
  expect(selects.length).toBeGreaterThan(20);
  for (const [name, disabled] of selects) {
    expect(disabled, name).toBe(false);
    expect(fields[name], name).toBeDefined();
  }

  await toggle(page).click();
  await expect(manual(page)).toBeVisible();
  for (const [name, value] of Object.entries(SAVED)) {
    await expect(page.locator(`select[name="${name}"]`), name).toHaveValue(value);
  }
  expect((await formFields(page)).COLLECTION_MODE).toBe('manual');
});

test('Re-check hardware works after switching to manual', async ({ page }) => {
  const requests = await openPage(page, 'page-dynamic.html');
  await page.evaluate(() => updateServiceStatus(true));
  const btn = page.locator('#uma-hw-recheck-btn');
  const rechecks = () => requests.filter((r) => r.action === 'capabilities' && r.refresh === '1').length;

  await expect(btn).toBeHidden();
  await toggle(page).click();
  await expect(btn).toBeVisible();
  await expect(btn).toBeEnabled();
  const before = rechecks();
  await btn.click();
  await expect.poll(rechecks).toBe(before + 1);
  await expect(btn).toHaveValue('Re-check hardware');
  await expect(page.locator('#uma-hw-status')).toContainText('Detected hardware: UPS');
});

test('MQTT command and telemetry controls round-trip independently', async ({ page }) => {
  await openPage(page, 'page-mqtt.html');
  const commands = page.getByRole('switch', { name: 'Disable MQTT Commands' });
  const discovery = page.getByRole('switch', { name: 'Home Assistant Auto-Discovery' });
  await expect(commands).toBeChecked();
  expect((await commands.boundingBox()).y).toBeGreaterThan((await discovery.boundingBox()).y);
  const topics = page.getByRole('group', { name: 'MQTT Telemetry' });
  await expect(topics).toBeVisible();
  const docker = topics.getByRole('checkbox', { name: 'Docker containers' });
  const datasets = topics.getByRole('checkbox', { name: 'ZFS datasets' });
  const psu = topics.getByRole('checkbox', { name: 'Power supply' });
  await expect(docker).not.toBeChecked();
  await expect(datasets).not.toBeChecked();
  await expect(psu).toBeChecked();

  await docker.check();
  await psu.uncheck();
  let fields = await formFields(page);
  expect(fields.MQTT_DISABLED_TOPICS).toBe('psu,zfs/datasets');
  expect(fields.MQTT_DISABLE_COMMANDS).toBe('true');

  await commands.press('Space');
  fields = await formFields(page);
  expect(fields.MQTT_DISABLE_COMMANDS).toBe('false');
  expect(fields.MQTT_DISABLED_TOPICS).toBe('psu,zfs/datasets');
  expect(fields.MQTT_HOME_ASSISTANT).toBe('true');
});

test('unrelated Apply preserves absent MQTT overrides and each edit saves independently', async ({ page }) => {
  await openPage(page, 'page.html');
  await page.locator('#uma_port_input').fill('8044');
  let fields = await formFields(page);
  expect(fields.MQTT_DISABLE_COMMANDS).toBeUndefined();
  expect(fields.MQTT_DISABLED_TOPICS).toBeUndefined();

  await page.getByRole('switch', { name: 'Enable MQTT', exact: true }).press('Space');
  const powerSupply = page.getByRole('checkbox', { name: 'Power supply' });
  await powerSupply.uncheck();
  fields = await formFields(page);
  expect(fields.MQTT_DISABLED_TOPICS).toBe('psu');
  expect(fields.MQTT_DISABLE_COMMANDS).toBeUndefined();

  await powerSupply.check();
  await page.getByRole('switch', { name: 'Disable MQTT Commands' }).press('Space');
  fields = await formFields(page);
  expect(fields.MQTT_DISABLED_TOPICS).toBe('');
  expect(fields.MQTT_DISABLE_COMMANDS).toBe('true');
});
