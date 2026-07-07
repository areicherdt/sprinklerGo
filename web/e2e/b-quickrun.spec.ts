import { expect, test } from '@playwright/test'

test('Schnellstart läuft und lässt sich stoppen', async ({ page }) => {
  await page.goto('/quickrun')
  const card = page.locator('.card', { hasText: 'Eigene Laufzeiten' })
  await card.locator('input[type="number"]').first().fill('3')
  await card.getByRole('button', { name: 'Starten' }).click()

  // Der Start navigiert zur Übersicht; der Lauf erscheint per SSE.
  await expect(page.locator('.pill', { hasText: 'Bewässerung läuft' })).toBeVisible()
  await expect(page.locator('.hero-number')).toContainText('Zone 1')

  await page.getByRole('button', { name: 'Alles stoppen' }).click()
  await expect(page.getByText('Keine Zone aktiv.')).toBeVisible()
})
