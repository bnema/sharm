const { test, expect } = require('@playwright/test');

const fixturePath = '/fixtures/h264-aac.mp4';

test('publishes direct H.264/AAC and server-fallback video uploads', async ({ page }) => {
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
  await page.locator('input[name="file"]').setInputFiles('/fixtures/server-fallback.webm');
  await page.getByRole('button', { name: 'Upload' }).click();

  await expect(page).toHaveURL(/\/$/, { timeout: 60_000 });
  await expect(page.getByRole('link', { name: 'server-fallback.webm' })).toBeVisible({ timeout: 60_000 });
});
