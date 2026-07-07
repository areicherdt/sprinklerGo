import { expect, test } from '@playwright/test'

test('Programm anlegen mit Startzeit und Laufzeit', async ({ page }) => {
  await page.goto('/schedules')
  await page.getByRole('button', { name: 'Neues Programm' }).click()

  await page.locator('.card').first().locator('input[type="text"]').fill('E2E Programm')

  const wann = page.locator('.card', { hasText: 'Startzeiten' })
  await wann.locator('input[type="checkbox"]').first().check()
  await wann.locator('input[type="time"]').first().fill('06:30')

  await page.getByLabel('Laufzeit Zone 1').fill('5')
  await expect(page.getByText('Gesamt:')).toContainText('5 min')
  await expect(page.getByText('Nächste Läufe:')).not.toContainText('keine')

  await page.getByRole('button', { name: 'Speichern', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('Programm angelegt.')
  await expect(page.locator('.card', { hasText: 'E2E Programm' })).toContainText('06:30')
})
