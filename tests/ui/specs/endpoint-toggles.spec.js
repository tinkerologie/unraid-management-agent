// UI tests for the endpoint switches on the plugin settings page (#273).
// global-setup.js renders page.html without a config (all on) and
// page-endpoints-off.html with all three off; every network call is mocked.
const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const BASE = 'http://uma.test';
const RENDER = path.join(__dirname, '..', '.render');
const JQUERY = path.join(__dirname, '..', 'node_modules', 'jquery', 'dist', 'jquery.min.js');

const TOGGLES = [
  { id: 'mcp_enabled', field: 'MCP_ENABLED' },
  { id: 'metrics_enabled', field: 'METRICS_ENABLED' },
  { id: 'websocket_enabled', field: 'WEBSOCKET_ENABLED' },
];

async function openPage(page, html) {
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/') {
      return route.fulfill({ contentType: 'text/html', body: fs.readFileSync(path.join(RENDER, html), 'utf8') });
    }
    if (url.pathname === '/jquery.js') {
      return route.fulfill({ contentType: 'application/javascript', body: fs.readFileSync(JQUERY, 'utf8') });
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
  });
  await page.goto(`${BASE}/`);
}

const postedFields = (page, id) =>
  page.locator(`#${id}`).evaluate((el) => Object.fromEntries(new FormData(el.closest('form'))));

for (const { id, field } of TOGGLES) {
  test(`${field} defaults to on and its switch updates the posted value`, async ({ page }) => {
    await openPage(page, 'page.html');
    await expect(page.locator(`#${id}`)).toBeChecked();
    expect((await postedFields(page, id))[field]).toBe('true');

    await page.locator(`label:has(#${id})`).click();
    await expect(page.locator(`#${id}`)).not.toBeChecked();
    expect((await postedFields(page, id))[field]).toBe('false');

    await page.locator(`label:has(#${id})`).click();
    expect((await postedFields(page, id))[field]).toBe('true');
  });

  test(`${field} renders off from a saved false`, async ({ page }) => {
    await openPage(page, 'page-endpoints-off.html');
    await expect(page.locator(`#${id}`)).not.toBeChecked();
    expect((await postedFields(page, id))[field]).toBe('false');
  });
}

test('the status panel lists only enabled endpoints', async ({ page }) => {
  await openPage(page, 'page.html');
  const endpoints = page.locator('#uma-endpoints');
  await expect(endpoints).toContainText('/api/v1/ws');
  await expect(endpoints).toContainText('/mcp');

  await openPage(page, 'page-endpoints-off.html');
  await expect(endpoints).toContainText('/api/v1');
  await expect(endpoints).not.toContainText('/api/v1/ws');
  await expect(endpoints).not.toContainText('/mcp');
});
