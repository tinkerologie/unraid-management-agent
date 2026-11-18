// Set UMA_DYNAMIX_STYLES to the styles directory from an Unraid 7.4.0 host.
const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const styles = process.env.UMA_DYNAMIX_STYLES;
const BASE = 'http://uma.test';
const render = path.join(__dirname, '..', '.render', 'page-mqtt.html');
const jquery = path.join(__dirname, '..', 'node_modules', 'jquery', 'dist', 'jquery.min.js');

const tools = [
  { name: 'get_health_status', category: 'System', description: 'Read system health.', read_only: true },
  {
    name: 'restart_container',
    category: 'Docker',
    description: 'Restart a fixture container. This long description verifies wrapping inside the local scroll area.',
    read_only: false,
    destructive: false,
  },
];

function weightedRgbBrightness(rgb) {
  const components = rgb.match(/\d+/g).slice(0, 3).map(Number);
  return 0.2126 * components[0] + 0.7152 * components[1] + 0.0722 * components[2];
}

async function openSettings(page, theme, catalogReady = Promise.resolve()) {
  const css = theme ? ['default-color-palette.css', 'default-base.css', `themes/${theme}.css`] : [];
  const html = fs.readFileSync(render, 'utf8')
    .replace('<html>', `<html class="Theme--${theme}">`)
    .replace('</head>', `${css.map((file) => `<link rel="stylesheet" href="/styles/${file}">`).join('')}</head>`);
  await page.route('**/*', async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname === '/') return route.fulfill({ contentType: 'text/html', body: html });
    if (pathname === '/jquery.js') return route.fulfill({ path: jquery, contentType: 'application/javascript' });
    const stylesheet = pathname.slice('/styles/'.length);
    if (pathname.startsWith('/styles/') && css.includes(stylesheet)) {
      return route.fulfill({ path: path.join(styles, stylesheet), contentType: 'text/css' });
    }
    if (pathname.endsWith('/include/exec.php')) return route.fulfill({ json: { running: true } });
    if (pathname.endsWith('/include/diagnostics.php')) {
      const action = new URL(route.request().url()).searchParams.get('action') ||
        new URLSearchParams(route.request().postData() || '').get('action');
      if (action === 'get_tool_policy') await catalogReady;
      const result = action === 'get_tool_policy' ? { tools, policies: {} } : { items: [] };
      return route.fulfill({ json: result });
    }
    return route.fulfill({ status: 404, body: '' });
  });
  await page.goto(BASE);
}

async function mutedTextContrast(page) {
  return page.locator('.uma-muted:visible').evaluateAll((elements) => {
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 1;
    const context = canvas.getContext('2d', { willReadFrequently: true });
    const fill = (color) => {
      context.fillStyle = color;
      context.fillRect(0, 0, 1, 1);
    };
    const luminance = () => {
      const channels = context.getImageData(0, 0, 1, 1).data.slice(0, 3);
      const [r, g, b] = Array.from(channels, (channel) => {
        const c = channel / 255;
        return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    };
    return elements.map((element) => {
      // Composite the stock and plugin backgrounds underneath each sample.
      const layers = [];
      for (let node = element; node; node = node.parentElement) {
        layers.push(getComputedStyle(node).backgroundColor);
      }
      fill('white');
      layers.reverse().forEach(fill);
      const background = luminance();
      fill(getComputedStyle(element).color);
      const foreground = luminance();
      return {
        name: element.textContent.trim().slice(0, 60),
        ratio: (Math.max(background, foreground) + 0.05) / (Math.min(background, foreground) + 0.05),
      };
    });
  });
}

test('settings rows associate labels with their controls', async ({ page }) => {
  await openSettings(page);
  const rows = await page.locator('dl.uma-settings > dt').count();
  expect(rows).toBeGreaterThan(40);
  await expect(page.locator('dl.uma-settings > dt + dd')).toHaveCount(rows);
  for (const id of [
    'uma_port_input', 'mcp_enabled', 'api_token', 'read_only',
    'mqtt_ha_enabled', 'debug_logging', 'collection_mode', 'uma-interval-system',
  ]) {
    const label = page.locator(`dt label[for="${id}"]`);
    const control = page.locator(`dd #${id}`);
    await expect(label).toBeVisible();
    await expect(control).toBeVisible();
    expect(await label.evaluate((element) => element.control?.id)).toBe(id);
  }
  const password = page.locator('#mqtt_password');
  await expect(password).toHaveValue('');
  await expect(password).toHaveAttribute('placeholder', '••••••••');

  const direct = page.locator('#uma-mcp-direct-url-item');
  const standard = page.locator('#uma-mcp-standard-url-link');
  await expect(direct).toBeHidden();
  await expect(standard).toContainText('/mcp');
  await page.locator('#mcp_connect_secret').fill('fixture-secret-only');
  await expect(direct).toBeVisible();
  await expect(direct).toContainText('/mcp/fixture-secret-only');
  await page.locator('#mcp_connect_secret').fill('');
  await expect(direct).toBeHidden();
});

for (const theme of ['white', 'black']) {
  test(`Dynamix ${theme} theme keeps settings usable at desktop and narrow widths`, async ({ page }) => {
    test.skip(!styles, 'requires the stock Dynamix styles');
    await page.setViewportSize({ width: 1440, height: 940 });
    await openSettings(page, theme);

    const body = page.locator('body');
    const colors = await body.evaluate((el) => {
      const css = getComputedStyle(el);
      return { foreground: css.color, background: css.backgroundColor };
    });
    const bg = weightedRgbBrightness(colors.background);
    const fg = weightedRgbBrightness(colors.foreground);
    expect(theme === 'white' ? bg - fg : fg - bg).toBeGreaterThan(160);

    const firstRow = page.locator('dl.uma-settings').first();
    const port = page.locator('#uma_port_input');
    const leftInset = async () => (await port.boundingBox()).x - (await firstRow.boundingBox()).x;
    expect(await leftInset()).toBeLessThan(250);
    await page.setViewportSize({ width: 2000, height: 940 });
    expect(await leftInset()).toBeLessThan(250);
    await page.setViewportSize({ width: 1440, height: 940 });

    const discovery = page.getByRole('switch', { name: 'Home Assistant Auto-Discovery' });
    await expect(discovery).toBeChecked();
    const track = page.locator('#mqtt_ha_enabled + .uma-switch-track');
    expect((await track.boundingBox()).width).toBeGreaterThanOrEqual(44);
    await discovery.focus();
    await expect(track).toHaveCSS('outline-style', 'solid');
    await discovery.press('Space');
    await expect(page.locator('#mqtt_ha_val')).toHaveValue('false');
    await expect(page.locator('#mqtt_enabled_val')).toHaveValue('true');
    await page.locator('#uma-interval-system').selectOption('30');
    const fields = await page.locator('#agentConfigForm').evaluate((form) => Object.fromEntries(new FormData(form)));
    expect(fields.INTERVAL_SYSTEM).toBe('30');
    expect(fields['#file']).toBe('unraid-management-agent/config.cfg');
    expect(fields.API_TOKEN).toBe('');

    if (process.env.UMA_DYNAMIX_SCREENSHOTS) {
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({ path: path.join(process.env.UMA_DYNAMIX_SCREENSHOTS, `settings-${theme}.png`) });
    }

    await page.setViewportSize({ width: 390, height: 840 });
    const portLabel = await page.locator('label[for="uma_port_input"]').boundingBox();
    const portControl = await page.locator('#uma_port_input').boundingBox();
    expect(portControl.y).toBeGreaterThan(portLabel.y);

    await page.locator('#uma-btn-toggle-policies').click();
    await expect(page.locator('#uma-tool-policy-tbody tr')).toHaveCount(tools.length);
    const dims = await page.evaluate(() => {
      const policy = document.querySelector('.uma-policy-table-wrap');
      return { width: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth,
        policyWidth: policy.clientWidth, policyScroll: policy.scrollWidth };
    });
    expect(dims.scroll).toBeLessThanOrEqual(dims.width);
    expect(dims.policyScroll).toBeGreaterThan(dims.policyWidth);
  });

  test(`Dynamix ${theme} theme gives muted settings text at least 4.5:1 contrast`, async ({ page }) => {
    test.skip(!styles, 'requires the stock Dynamix styles');
    let releaseCatalog;
    const catalogReady = new Promise((resolve) => { releaseCatalog = resolve; });
    await openSettings(page, theme, catalogReady);

    const samples = await mutedTextContrast(page);
    expect(samples.some(({ name }) => name.startsWith('Recommended:'))).toBe(true);
    const panel = page.locator('#uma-tool-policy-panel');
    await page.locator('#uma-btn-toggle-policies').click();
    try {
      await expect(panel.getByText('Loading MCP tool catalog…')).toBeVisible();
      samples.push(...await mutedTextContrast(page));
    } finally {
      releaseCatalog();
    }
    await expect(panel.getByText('Docker', { exact: true }).last()).toBeVisible();
    samples.push(...await mutedTextContrast(page));

    await page.locator('#uma-policy-search').fill('no matching tools');
    await expect(panel.getByText('No tools match the selected filters.')).toBeVisible();
    samples.push(...await mutedTextContrast(page));

    for (const text of ['Loading MCP tool catalog…', 'System', 'Docker', 'No tools match the selected filters.']) {
      expect.soft(samples.some(({ name }) => name === text), text).toBe(true);
    }
    const min = Math.min(...samples.map(({ ratio }) => ratio));
    console.log(`Dynamix ${theme} minimum muted-text contrast ${min.toFixed(2)}:1`);
    for (const { name, ratio } of samples) {
      expect.soft(ratio, name).toBeGreaterThanOrEqual(4.5);
    }
  });
}
