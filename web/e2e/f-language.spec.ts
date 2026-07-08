import { expect, test } from '@playwright/test'

// Läuft nach den anderen Specs (alphabetisch f-*) und setzt am Ende wieder
// auf Deutsch zurück, damit der geteilte Server-Zustand konsistent bleibt.
test('Sprache auf Englisch umschalten und zurück', async ({ page }) => {
  await page.goto('/settings')

  const langSelect = page.locator('.card', { hasText: 'Sprache' }).locator('select').last()
  await langSelect.selectOption('en')
  await page.getByRole('button', { name: 'Speichern', exact: true }).click()

  // Der Sprachwechsel lädt die Seite neu; die Navigation ist jetzt englisch.
  await expect(page.getByRole('link', { name: 'Overview' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Settings' })).toBeVisible()

  // Dashboard prüfen.
  await page.getByRole('link', { name: 'Overview' }).click()
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await expect(page.locator('.card', { hasText: 'Rain delay' })).toBeVisible()

  // Zurück auf Deutsch.
  await page.getByRole('link', { name: 'Settings' }).click()
  const langSelectEn = page.locator('.card', { hasText: 'Sprache' }).locator('select').last()
  await langSelectEn.selectOption('de')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByRole('link', { name: 'Übersicht' })).toBeVisible()
})
