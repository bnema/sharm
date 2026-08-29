const { test, expect } = require('@playwright/test');

const fixturePath = '/fixtures/h264-aac.mp4';

test('publishes direct, client-encoded, and server-fallback video uploads', async ({ page }) => {
  await page.goto('/setup');
  await page.getByPlaceholder('Username').fill('e2e-user');
  await page.getByPlaceholder('Password', { exact: true }).fill('E2e-Password123!');
  await page.getByPlaceholder('Confirm password').fill('E2e-Password123!');
  await page.getByRole('button', { name: 'Create Account' }).click();
  await expect(page).toHaveURL(/\/$/);

  await page.goto('/upload');
  await page.locator('input[name="file"]').setInputFiles(fixturePath);
  await page.getByRole('button', { name: 'Upload' }).click();

  await expect(page).toHaveURL(/\/$/, { timeout: 60_000 });
  await expect(page.getByRole('link', { name: 'h264-aac.mp4' })).toBeVisible();
  await expect(page.locator('body')).toContainText(/H264|h264/);

  await page.goto('/upload');
  const canEncodeClientOutput = await page.evaluate(async () => {
    if (
      !globalThis.isSecureContext ||
      typeof VideoEncoder === 'undefined' ||
      typeof AudioEncoder === 'undefined'
    ) return false;
    try {
      const [videoSupport, audioSupport] = await Promise.all([
        VideoEncoder.isConfigSupported({
          codec: 'avc1.42001f',
          width: 320,
          height: 180,
          bitrate: 1_000_000,
          framerate: 30,
        }),
        AudioEncoder.isConfigSupported({
          codec: 'mp4a.40.2',
          sampleRate: 48_000,
          numberOfChannels: 1,
          bitrate: 128_000,
        }),
      ]);
      return videoSupport.supported === true && audioSupport.supported === true;
    } catch (_) {
      return false;
    }
  });
  const finalizePayloads = [];
  await page.route('**/upload/session/*/assets/*/complete', async (route) => {
    const response = await route.fetch();
    const body = await response.body();
    let payload = null;
    try {
      payload = JSON.parse(body.toString('utf8'));
    } catch (_) {
      // The assertion below reports a missing payload without breaking delivery to the page.
    }
    finalizePayloads.push({ url: route.request().url(), payload });
    await route.fulfill({ response, body });
  });
  await page.locator('input[name="file"]').setInputFiles('/fixtures/client-encoding.webm');
  const clientSessionRequest = page.waitForRequest(
    (request) => request.method() === 'POST' && request.url().endsWith('/upload/session'),
  );
  const preparationStatusPromise = page.waitForFunction(() => {
    const text = document.querySelector('#result')?.textContent || '';
    return /uploaded directly|Client-side H\.264 encoding complete|using the server fallback/.test(text) ? text : false;
  });
  await page.getByRole('button', { name: 'Upload' }).click();

  const clientPayload = (await clientSessionRequest).postDataJSON();
  const preparationStatus = await (await preparationStatusPromise).jsonValue();
  expect(clientPayload.primary_size).toBeGreaterThan(0);
  if (canEncodeClientOutput) {
    expect(clientPayload.primary_filename, preparationStatus || 'missing preparation status').toBe('client-encoding.mp4');
    await expect.poll(
      () => finalizePayloads.some((entry) => entry.payload?.variant),
      { message: 'wait for primary finalize response', timeout: 60_000 },
    ).toBe(true);
    const primaryFinalize = finalizePayloads.find((entry) => entry.payload?.variant);
    expect(primaryFinalize, 'no primary finalize response captured').toBeTruthy();
    expect(primaryFinalize.payload.variant).toMatchObject({
      origin: 'client',
      video_codec: 'h264',
      audio_codec: 'aac',
      status: 'done',
    });
  } else {
    expect(clientPayload.primary_filename).toBe('client-encoding.webm');
    expect(preparationStatus).toContain('using the server fallback');
  }
  await expect(page).toHaveURL(/\/$/, { timeout: 60_000 });
  await expect(page.getByRole('link', { name: 'client-encoding.webm' })).toBeVisible({ timeout: 60_000 });

  await page.route('**/client-video-worker.js', (route) => route.abort());
  await page.goto('/upload');
  await page.locator('input[name="file"]').setInputFiles('/fixtures/client-encoding.webm');
  await page.locator('#keep-original').check();
  const fallbackSessionRequest = page.waitForRequest(
    (request) => request.method() === 'POST' && request.url().endsWith('/upload/session'),
  );
  await page.getByRole('button', { name: 'Upload' }).click();

  const fallbackPayload = (await fallbackSessionRequest).postDataJSON();
  expect(fallbackPayload).toMatchObject({
    keep_original: true,
    reuse_primary_as_original: true,
    original_size: 0,
  });
  await expect(page).toHaveURL(/\/$/, { timeout: 60_000 });
  await expect(page.getByRole('link', { name: 'client-encoding.webm' })).toHaveCount(2);
});
