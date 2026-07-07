import { expect, test } from '@playwright/test'

test('Regenpause aktivieren und aufheben', async ({ page }) => {
  await page.goto('/')
  const card = page.locator('.card', { hasText: 'Regenpause' })
  await card.getByRole('button', { name: '48 h' }).click()
  await expect(card).toContainText('Aktiv bis')
  await expect(page.locator('.toast')).toContainText('Regenpause für 48 h aktiviert.')
  await card.getByRole('button', { name: 'Aufheben' }).click()
  await expect(card.getByRole('button', { name: '24 h' })).toBeVisible()
})
