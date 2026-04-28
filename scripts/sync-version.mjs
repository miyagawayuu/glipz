#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const packagePath = path.join(root, "web", "package.json");
const webAppInfoPath = path.join(root, "web", "src", "lib", "appInfo.ts");
const backendAppInfoPath = path.join(root, "backend", "internal", "httpserver", "app_info.go");

const packageJson = JSON.parse(fs.readFileSync(packagePath, "utf8"));
const version = String(packageJson.version || "").trim();

if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) {
  throw new Error(`web/package.json has an invalid app version: ${JSON.stringify(packageJson.version)}`);
}

function replaceOnce(filePath, pattern, replacement) {
  const before = fs.readFileSync(filePath, "utf8");
  if (!pattern.test(before)) {
    throw new Error(`version pattern not found in ${path.relative(root, filePath)}`);
  }
  const after = before.replace(pattern, replacement);
  fs.writeFileSync(filePath, after);
}

replaceOnce(webAppInfoPath, /export const APP_VERSION = "([^"]+)";/, `export const APP_VERSION = "${version}";`);
replaceOnce(backendAppInfoPath, /glipzAppVersion\s+= "([^"]+)"/, `glipzAppVersion              = "${version}"`);

console.log(`Synced Glipz app version ${version}`);
