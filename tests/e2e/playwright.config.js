const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: '.',
  timeout: 60_000,
  retries: 0,
  workers: 1,
  use: {
    baseURL: process.env.SHARM_BASE_URL || 'http://sharm:7890',
    browserName: 'chromium',
    trace: 'retain-on-failure',
  },
});
