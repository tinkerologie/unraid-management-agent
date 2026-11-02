// UI tests for the MCP tool policy panel (#307). Every network call is mocked.
const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const BASE = 'http://uma.test';
const RENDER = path.join(__dirname, '..', '.render');
const JQUERY = path.join(__dirname, '..', 'node_modules', 'jquery', 'dist', 'jquery.min.js');

const LONG = 'Reads a very long description that has to wrap onto more than one line so that nobody needs to hover over it, including people using a touch screen. '.repeat(3);
const TOOLS = [
  { name: 'get_system_info', category: 'System', read_only: true, description: LONG },
  { name: 'system_reboot', category: 'System', read_only: false, description: 'Reboots the server.' },
];

// Opens the panel and returns the requests that would save policies.
async function openPanel(page) {
  const saves = [];
  await page.setViewportSize({ width: 900, height: 800 });
  await page.route('**/*', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname === '/') {
      return route.fulfill({ contentType: 'text/html', body: fs.readFileSync(path.join(RENDER, 'page.html'), 'utf8') });
    }
    if (url.pathname === '/jquery.js') {
      return route.fulfill({ contentType: 'application/javascript', body: fs.readFileSync(JQUERY, 'utf8') });
    }
    if (url.searchParams.get('action') === 'get_tool_policy') {
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tools: TOOLS, policies: {} }) });
    }
    if (request.method() !== 'GET' && (request.postData() || '').includes('polic')) saves.push(request.url());
    return route.fulfill({ contentType: 'application/json', body: '{}' });
  });
  await page.goto(`${BASE}/`);
  await page.locator('#uma-btn-toggle-policies').click();
  await expect(page.locator('.uma-policy-select')).toHaveCount(TOOLS.length);
  return saves;
}

test('All → Hidden hides every tool and saves nothing until applied', async ({ page }) => {
  const saves = await openPanel(page);
  await expect(page.locator('#uma-btn-save-policies')).toBeHidden();

  await page.locator('input[type="button"][value="All → Hidden"]').click();

  await expect(page.locator('.uma-policy-select').first()).toHaveValue('hidden');
  await expect(page.locator('.uma-policy-select').last()).toHaveValue('hidden');
  expect(JSON.parse(await page.locator('#tool_policy_json').inputValue())).toEqual({
    get_system_info: 'hidden',
    system_reboot: 'hidden',
  });
  await expect(page.locator('#uma-btn-save-policies')).toBeVisible();
  expect(saves).toEqual([]);
});

test('Reset All stays the last bulk action and undoes All → Hidden', async ({ page }) => {
  await openPanel(page);
  await page.locator('input[type="button"][value="All → Hidden"]').click();
  const buttons = page.locator('#uma-tool-policy-panel input[type="button"]');
  await expect(buttons.last()).toHaveValue('Reset All');

  await buttons.last().click();
  expect(JSON.parse(await page.locator('#tool_policy_json').inputValue())).toEqual({});
});

test('a long description wraps in full without widening the table', async ({ page }) => {
  await openPanel(page);
  const cell = page.locator('#uma-tool-policy-tbody td', { hasText: 'very long description' });
  await expect(cell).toContainText(LONG.trim());
  const shape = await cell.evaluate((el) => ({
    height: el.getBoundingClientRect().height,
    clipped: el.scrollWidth > el.clientWidth,
    overflow: getComputedStyle(el).textOverflow,
  }));
  const short = await page.locator('#uma-tool-policy-tbody td', { hasText: 'Reboots the server.' }).boundingBox();
  expect(shape.height).toBeGreaterThan(1.5 * short.height);
  expect(shape.clipped).toBe(false);
  expect(shape.overflow).not.toBe('ellipsis');
  const table = await page.locator('#uma-tool-policy-table').boundingBox();
  const panel = await page.locator('#uma-tool-policy-panel').boundingBox();
  expect(table.width).toBeLessThanOrEqual(panel.width);
});
