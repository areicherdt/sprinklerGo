import { expect, test } from '@playwright/test'

test('Zone umbenennen und speichern', async ({ page }) => {
  await page.goto('/zones')
  const name = page.getByLabel('Name Zone 1', { exact: true })
  await name.fill('Rasen vorne')
  await page.getByRole('button', { name: 'Speichern' }).first().click()
  await expect(page.locator('.toast')).toContainText('Zone gespeichert.')

  await page.reload()
  await expect(page.getByLabel('Name Zone 1', { exact: true })).toHaveValue('Rasen vorne')
})
