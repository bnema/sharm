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
  const canEncodeH264 = await page.evaluate(async () => {
    if (!globalThis.isSecureContext || typeof VideoEncoder === 'undefined') return false;
    try {
      const support = await VideoEncoder.isConfigSupported({
        codec: 'avc1.42001f',
        width: 320,
        height: 180,
        bitrate: 1_000_000,
        framerate: 30,
      });
      return support.supported === true;
    } catch (_) {
      return false;
    }
  });
  await page.locator('input[name="file"]').setInputFiles('/fixtures/client-encoding.webm');
  const clientSessionRequest = page.waitForRequest(
    (request) => request.method() === 'POST' && request.url().endsWith('/upload/session'),
  );
  await page.getByRole('button', { name: 'Upload' }).click();

  const clientPayload = (await clientSessionRequest).postDataJSON();
  const preparationStatus = await page.locator('#result').textContent();
  expect(clientPayload.primary_size).toBeGreaterThan(0);
  if (canEncodeH264) {
    expect(clientPayload.primary_filename, preparationStatus || 'missing preparation status').toBe('client-encoding.mp4');
  } else {
    expect(clientPayload.primary_filename).toBe('client-encoding.webm');
    expect(preparationStatus).toContain('using the server fallback');
  }
  await expect(page).toHaveURL(/\/$/, { timeout: 60_000 });
  await expect(page.getByRole('link', { name: 'client-encoding.webm' })).toBeVisible({ timeout: 60_000 });

  if (canEncodeH264) {
    await page.route('**/client-video-worker.js', (route) => route.abort());
    await page.goto('/upload');
    await page.locator('input[name="file"]').setInputFiles('/fixtures/client-encoding.webm');
    await page.getByRole('button', { name: 'Upload' }).click();

    await expect(page).toHaveURL(/\/$/, { timeout: 60_000 });
    await expect(page.getByRole('link', { name: 'client-encoding.webm' })).toHaveCount(2);
  }
});
