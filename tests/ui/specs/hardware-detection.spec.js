// UI tests for hardware auto-detection on the plugin settings page (#261).
// The page is rendered by global-setup.js; every network call is mocked here.
const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const BASE = 'http://uma.test';
const PAGE_HTML = path.join(__dirname, '..', '.render', 'page.html');
const JQUERY = path.join(__dirname, '..', 'node_modules', 'jquery', 'dist', 'jquery.min.js');

const HARDWARE_SELECTS = [
  'INTERVAL_PSU',
  'INTERVAL_GPU',
  'INTERVAL_NIC',
  'INTERVAL_UPS',
  'INTERVAL_NUT',
  'INTERVAL_ZFS',
  'INTERVAL_STORAGE_TOPOLOGY',
];

// The reference server of #261: an Intel iGPU and an APC UPS, nothing else.
const REFERENCE = {
  'hardware.psu': 'absent',
  'hardware.gpu': 'present',
  'hardware.ups': 'present',
  'hardware.nic_optics': 'absent',
  'hardware.nut': 'absent',
  'hardware.zfs': 'absent',
  'hardware.storage_topology': 'absent',
  'hardware.fan_control': 'present',
};

function capabilities(states) {
  return {
    unraid_version: '7.3.3',
    checked_at: '2026-10-14T09:00:00Z',
    items: [
      { name: 'docker', available: true, state: 'present', target: '/usr/bin/docker' },
      ...Object.entries(states).map(([name, state]) => ({
        name,
        available: state === 'present',
        state,
        detail: `${name} is ${state}`,
      })),
    ],
  };
}

const json = (body, status = 200) => ({ status, contentType: 'application/json', body: JSON.stringify(body) });

// Opens the page. respond(form, n) answers the n-th capabilities request
// (form holds the posted fields) with a route.fulfill() option object, or a
// promise of one. Returns the list of capabilities requests made.
async function openPage(page, respond) {
  const requests = [];
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/') {
      return route.fulfill({ contentType: 'text/html', body: fs.readFileSync(PAGE_HTML, 'utf8') });
    }
    if (url.pathname === '/jquery.js') {
      return route.fulfill({ contentType: 'application/javascript', body: fs.readFileSync(JQUERY, 'utf8') });
    }
    if (url.pathname.endsWith('/include/diagnostics.php')) {
      const form = Object.fromEntries(new URLSearchParams(route.request().postData() || ''));
      if (form.action !== 'capabilities') return route.fulfill(json({ error: 'unexpected' }, 400));
      requests.push(form);
      return route.fulfill(await respond(form, requests.length));
    }
    if (url.pathname.endsWith('/include/exec.php')) {
      return route.fulfill(json({ running: true }));
    }
    return route.fulfill({ status: 404, body: '' });
  });
  await page.goto(`${BASE}/`);
  await expect.poll(() => requests.length).toBeGreaterThan(0);
  return requests;
}

const row = (page, capability) => page.locator(`.uma-interval-row[data-uma-capability="${capability}"]`);
const status = (page) => page.locator('#uma-hw-status');

async function expectReferenceVisibility(page) {
  for (const [capability, state] of Object.entries(REFERENCE)) {
    if (capability === 'hardware.fan_control') continue; // no settings row
    if (state === 'absent') await expect(row(page, capability)).toBeHidden();
    else await expect(row(page, capability)).toBeVisible();
  }
}

test('absent hardware hides its row whatever the setting; present hardware is shown', async ({ page }) => {
  await openPage(page, () => json(capabilities(REFERENCE)));
  await expect(status(page)).toContainText('Detected hardware: GPU, UPS');
  await expect(status(page)).toContainText('last checked');
  await expectReferenceVisibility(page);

  // Disabled and an explicit interval are hidden too while the hardware is absent.
  for (const value of ['0', '300', 'auto']) {
    await page.evaluate((v) => {
      ['INTERVAL_NIC', 'INTERVAL_ZFS'].forEach((name) => {
        document.querySelector(`select[name="${name}"]`).value = v;
      });
      applyCapabilityVisibility();
    }, value);
    await expect(row(page, 'hardware.nic_optics')).toBeHidden();
    await expect(row(page, 'hardware.zfs')).toBeHidden();
  }
  await expect(row(page, 'hardware.nic_optics')).toHaveAttribute('title', /^Not detected: /);
  await expect(row(page, 'hardware.gpu')).toHaveAttribute('title', /^Detected: /);
});

test('Show all collectors reveals rows without changing any setting', async ({ page }) => {
  await openPage(page, () => json(capabilities(REFERENCE)));
  await expectReferenceVisibility(page);
  const values = () =>
    page.evaluate((names) => names.map((n) => document.querySelector(`select[name="${n}"]`).value), HARDWARE_SELECTS);
  const before = await values();

  await page.locator('label:has(#show_all_collectors)').click();
  await expect(page.locator('#show_all_collectors')).toBeChecked();
  await expect(page.locator('#show_all_collectors_val')).toHaveValue('true');
  for (const capability of Object.keys(REFERENCE).filter((c) => c !== 'hardware.fan_control')) {
    await expect(row(page, capability)).toBeVisible();
  }
  expect(await values()).toEqual(before);

  await page.locator('label:has(#show_all_collectors)').click();
  await expect(page.locator('#show_all_collectors')).not.toBeChecked();
  await expect(page.locator('#show_all_collectors_val')).toHaveValue('false');
  await expectReferenceVisibility(page);
  expect(await values()).toEqual(before);
});

test('fails open when detection is unavailable', async ({ page }) => {
  await openPage(page, () => json({ error: 'Could not reach the agent' }, 502));
  await expect(status(page)).toContainText('Hardware detection unavailable');
  for (const capability of Object.keys(REFERENCE).filter((c) => c !== 'hardware.fan_control')) {
    await expect(row(page, capability)).toBeVisible();
  }
});

test('unknown and missing hardware items keep their rows', async ({ page }) => {
  const states = { ...REFERENCE, 'hardware.nic_optics': 'unknown' };
  delete states['hardware.zfs'];
  await openPage(page, () => json(capabilities(states)));
  await expect(status(page)).toContainText('detection unavailable for some hardware');
  await expect(row(page, 'hardware.nic_optics')).toBeVisible();
  await expect(row(page, 'hardware.zfs')).toBeVisible();
  await expect(row(page, 'hardware.storage_topology')).toBeHidden();
});

test('Re-check hardware asks the agent to refresh and restores the button', async ({ page }) => {
  let release;
  const gate = new Promise((resolve) => {
    release = resolve;
  });
  const requests = await openPage(page, async (form, n) => {
    if (form.refresh === '1') await gate;
    return json(capabilities(n === 1 ? REFERENCE : { ...REFERENCE, 'hardware.zfs': 'present' }));
  });
  await page.evaluate(() => updateServiceStatus(true));
  const btn = page.locator('#uma-hw-recheck-btn');
  await expect(btn).toBeEnabled();

  await btn.click();
  await expect(btn).toHaveValue('Checking…');
  await expect(btn).toBeDisabled();
  release();
  await expect(btn).toHaveValue('Re-check hardware');
  await expect(btn).toBeEnabled();
  expect(requests.at(-1).refresh).toBe('1');
  expect(requests[0].refresh).toBe('0');
  await expect(row(page, 'hardware.zfs')).toBeVisible();
  await expect(status(page)).toContainText('ZFS pools');
});

test('a late response from an older check is ignored', async ({ page }) => {
  let releaseOld;
  const oldGate = new Promise((resolve) => {
    releaseOld = resolve;
  });
  const requests = await openPage(page, async (form, n) => {
    if (n === 2) {
      await oldGate; // the older check answers last: everything present
      return json(capabilities(Object.fromEntries(Object.keys(REFERENCE).map((k) => [k, 'present']))));
    }
    return json(capabilities(REFERENCE));
  });
  await expect(status(page)).toContainText('Detected hardware: GPU, UPS');

  await page.evaluate(() => {
    loadCapabilities(false); // request 2: delayed
    loadCapabilities(false); // request 3: answers at once
  });
  await expect.poll(() => requests.length).toBe(3);
  await expectReferenceVisibility(page);
  const oldResponse = page.waitForResponse((response) => response.url().endsWith('/include/diagnostics.php'));
  releaseOld();
  await oldResponse;
  // jQuery decrements $.active only after the request's callbacks have run.
  await page.waitForFunction(() => window.jQuery.active === 0);
  await expectReferenceVisibility(page);
  await expect(status(page)).toContainText('Detected hardware: GPU, UPS');
});

test('service start and stop keep Re-check hardware and detection in sync', async ({ page }) => {
  let agentUp = true;
  const requests = await openPage(page, () =>
    agentUp ? json(capabilities(REFERENCE)) : json({ error: 'not running' }, 502),
  );
  const btn = page.locator('#uma-hw-recheck-btn');

  agentUp = false;
  await page.evaluate(() => updateServiceStatus(false));
  await expect(btn).toBeDisabled();
  await expect.poll(() => requests.length).toBe(2);
  await expect(status(page)).toContainText('Hardware detection unavailable');
  await expect(row(page, 'hardware.nic_optics')).toBeVisible();

  agentUp = true;
  await page.evaluate(() => updateServiceStatus(true));
  await expect(btn).toBeEnabled();
  await expect.poll(() => requests.length).toBe(3);
  await expectReferenceVisibility(page);
});

test('only hardware selects offer Auto, selected by default', async ({ page }) => {
  await openPage(page, () => json(capabilities(REFERENCE)));
  for (const name of HARDWARE_SELECTS) {
    const select = page.locator(`select[name="${name}"]`);
    await expect(select).toHaveValue('auto');
    await expect(select.locator('option').first()).toHaveAttribute('value', 'auto');
  }
  const others = await page.evaluate(
    (names) =>
      Array.from(document.querySelectorAll('.uma-interval-select'))
        .filter((s) => !names.includes(s.name))
        .filter((s) => s.querySelector('option[value="auto"]'))
        .map((s) => s.name),
    HARDWARE_SELECTS,
  );
  expect(others).toEqual([]);
});

test('a section whose rows are all hidden is hidden', async ({ page }) => {
  await openPage(page, () => json(capabilities(REFERENCE)));
  const sections = await page.evaluate(() => {
    const section = document.createElement('div');
    section.className = 'uma-interval-section';
    section.id = 'test-section';
    section.innerHTML =
      '<div class="uma-interval-row" data-uma-capability="hardware.nic_optics"><select><option value="300">5 minutes</option></select></div>' +
      '<div class="uma-interval-row" data-uma-capability="hardware.storage_topology"><select><option value="auto">Auto</option></select></div>';
    document.querySelector('.uma-interval-section').parentNode.appendChild(section);
    applyCapabilityVisibility();
    return Array.from(document.querySelectorAll('.uma-interval-section')).map((s) => s.style.display);
  });
  await expect(page.locator('#test-section')).toBeHidden();
  // The page's own sections each keep a visible row.
  expect(sections.slice(0, -1).every((d) => d === '')).toBe(true);
});


test('PSU telemetry uses automatic detection and appears when its USB interface is present', async ({ page }) => {
  await openPage(page, () => json(capabilities({ ...REFERENCE, 'hardware.psu': 'present' })));
  await expect(row(page, 'hardware.psu')).toBeVisible();
  await expect(page.locator('select[name="INTERVAL_PSU"]')).toHaveValue('auto');
  await expect(status(page)).toContainText('PSU telemetry');
});
