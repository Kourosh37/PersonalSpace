import { expect, test, type Page } from "@playwright/test";

const now = new Date("2026-05-22T12:00:00.000Z").toISOString();

async function mockApi(page: Page) {
  await page.route("**/api/auth/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ user: { id: "user-1", username: "admin", role: "admin" } }),
    });
  });

  await page.route("**/api/folders/items**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: "folder-1",
            name: "Projects",
            type: "folder",
            modifiedAt: now,
            createdAt: now,
          },
          {
            id: "file-1",
            name: "report.pdf",
            type: "file",
            sizeBytes: 2048,
            mimeType: "application/pdf",
            extension: ".pdf",
            modifiedAt: now,
            createdAt: now,
          },
        ],
      }),
    });
  });

  await page.route("**/api/admin/settings/upload", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ mode: "custom", maxFileSizeBytes: 10 * 1024 * 1024 }),
    });
  });

  await page.route("**/api/admin/users", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: "user-1",
            username: "admin",
            role: "admin",
            isActive: true,
            usedStorageBytes: 4096,
            createdAt: now,
          },
        ],
      }),
    });
  });

  await page.route("**/api/public/shares/demo-token", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        id: "share-1",
        targetType: "folder",
        targetId: "folder-1",
        name: "Shared Folder",
        allowPreview: true,
        allowDownload: true,
        allowFolderBrowse: true,
        passwordRequired: false,
      }),
    });
  });

  await page.route("**/api/public/shares/demo-token/items**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: "file-2",
            name: "shared.txt",
            type: "file",
            sizeBytes: 128,
            modifiedAt: now,
          },
        ],
      }),
    });
  });
}

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test("login page renders and submits credentials", async ({ page }) => {
  await page.route("**/api/auth/login", async (route) => {
    const body = route.request().postDataJSON();
    expect(body).toEqual({ username: "admin", password: "password123" });
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ user: body }) });
  });

  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "Sign in to Space" })).toBeVisible();
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("password123");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/dashboard$/);
});

test("dashboard loads file manager and upload controls", async ({ page }) => {
  await page.goto("/dashboard");

  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
  await expect(page.getByText("Signed in as admin (admin)")).toBeVisible();
  await expect(page.getByRole("button", { name: "Projects" })).toBeVisible();
  await expect(page.getByText("report.pdf")).toBeVisible();
  await expect(page.getByRole("button", { name: "Upload Files" })).toBeVisible();
});

test("admin settings and users pages load with mocked API data", async ({ page }) => {
  await page.goto("/admin/settings/upload");
  await expect(page.getByRole("heading", { name: "Upload Settings" })).toBeVisible();
  await expect(page.getByText("Computed bytes: 10,485,760")).toBeVisible();

  await page.goto("/admin/users");
  await expect(page.getByRole("heading", { name: "Users" })).toBeVisible();
  await expect(page.locator("table input").first()).toHaveValue("admin");
});

test("public share page lists shared folder items", async ({ page }) => {
  await page.goto("/s/demo-token");

  await expect(page.getByRole("heading", { name: "Shared: Shared Folder" })).toBeVisible();
  await expect(page.getByText("shared.txt")).toBeVisible();
  await expect(page.getByRole("link", { name: "Download ZIP" })).toBeVisible();
});
