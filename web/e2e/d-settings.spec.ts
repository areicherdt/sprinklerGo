import { expect, test } from '@playwright/test'

test('Einstellungen speichern und wiederladen', async ({ page }) => {
  await page.goto('/settings')
  const card = page.locator('.card', { hasText: 'Saisonale Anpassung' })
  const input = card.locator('input[type="number"]').first()
  await input.fill('80')
  await page.getByRole('button', { name: 'Speichern', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('Einstellungen gespeichert.')

  await page.reload()
  await expect(card.locator('input[type="number"]').first()).toHaveValue('80')
})
