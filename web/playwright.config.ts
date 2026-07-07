import { defineConfig } from '@playwright/test'

// E2E gegen das echte Binary (Mock-Ausgang, frische Config/DB pro Lauf).
// Sequentiell, weil alle Tests denselben Server-Zustand teilen.
export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: 'http://localhost:8126',
  },
  webServer: {
    command:
      'rm -f /tmp/sprinklergo-e2e-config.json /tmp/sprinklergo-e2e.db* && exec ../bin/sprinklerd-e2e -config /tmp/sprinklergo-e2e-config.json -db /tmp/sprinklergo-e2e.db -port 8126',
    url: 'http://localhost:8126/api/state',
    reuseExistingServer: false,
    timeout: 15_000,
  },
})
