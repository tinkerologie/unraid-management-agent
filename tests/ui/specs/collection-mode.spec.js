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

const toggle = (page) => page.locator('label:has(#collection_mode)');
const manual = (page) => page.locator('#uma-manual-intervals');
const formFields = (page) =>
  page.locator('#collection_mode').evaluate((el) => Object.fromEntries(new FormData(el.closest('form'))));

// Everything under the toggle that Dynamic mode hides.
const MANUAL_ONLY = [
  'text=How often data is collected',
  'label:has(#show_all_collectors)',
  '#uma-hw-recheck-btn',
  '#uma-hw-status',
  'select[name="INTERVAL_SYSTEM"]',
  '.uma-power-warning:has-text("Power Tip")',
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
