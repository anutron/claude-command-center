#!/usr/bin/env node

import { google } from "googleapis";
import { CodeChallengeMethod } from "google-auth-library";
import { createServer } from "http";
import { URL } from "url";
import { readFileSync, writeFileSync, mkdirSync, existsSync, readdirSync } from "fs";
import { homedir } from "os";
import { join } from "path";
import { execFile } from "child_process";
import { randomBytes, createHash } from "crypto";

const SCOPES = [
  "https://www.googleapis.com/auth/gmail.modify",
  "https://www.googleapis.com/auth/gmail.compose",
];
const TOKEN_DIR = join(homedir(), ".gmail-mcp");
const REDIRECT_PORT = 3093;
const REDIRECT_URI = `http://127.0.0.1:${REDIRECT_PORT}`;

interface AccountData {
  clientId: string;
  clientSecret: string;
  tokens: Record<string, unknown>;
}

function accountPath(account: string): string {
  return join(TOKEN_DIR, `${account}.json`);
}

export function listAccounts(): string[] {
  if (!existsSync(TOKEN_DIR)) return [];
  return readdirSync(TOKEN_DIR)
    .filter((f) => f.endsWith(".json"))
    .map((f) => f.replace(".json", ""));
}

/** Generate PKCE code_verifier and S256 code_challenge per RFC 7636. */
function generatePKCE(): { codeVerifier: string; codeChallenge: string } {
  const codeVerifier = randomBytes(32)
    .toString("base64url"); // base64url, no padding
  const codeChallenge = createHash("sha256")
    .update(codeVerifier)
    .digest("base64url"); // base64url, no padding
  return { codeVerifier, codeChallenge };
}

function createOAuth2ClientWith(clientId: string, clientSecret: string) {
  return new google.auth.OAuth2(clientId, clientSecret, REDIRECT_URI);
}

export function loadCredentials(account: string): ReturnType<typeof createOAuth2ClientWith> | null {
  const path = accountPath(account);
  if (!existsSync(path)) return null;

  const data: AccountData = JSON.parse(readFileSync(path, "utf8"));

  // Support legacy format (tokens only, no clientId/clientSecret)
  const clientId = data.clientId || process.env.GMAIL_CLIENT_ID;
  const clientSecret = data.clientSecret || process.env.GMAIL_CLIENT_SECRET;

  if (!clientId || !clientSecret) {
    throw new Error(
      `Account "${account}" is missing OAuth credentials.\n` +
        `Re-auth with: GMAIL_CLIENT_ID=... GMAIL_CLIENT_SECRET=... npm run auth ${account}`
    );
  }

  const oauth2Client = createOAuth2ClientWith(clientId, clientSecret);
  oauth2Client.setCredentials(data.tokens || data as any);
  return oauth2Client;
}

function saveAccount(account: string, clientId: string, clientSecret: string, tokens: object) {
  if (!existsSync(TOKEN_DIR)) {
    mkdirSync(TOKEN_DIR, { recursive: true });
  }
  const data: AccountData = { clientId, clientSecret, tokens: tokens as Record<string, unknown> };
  writeFileSync(accountPath(account), JSON.stringify(data, null, 2), { mode: 0o600 });
}

async function runAuthFlow(account: string) {
  const clientId = process.env.GMAIL_CLIENT_ID;
  const clientSecret = process.env.GMAIL_CLIENT_SECRET;

  if (!clientId || !clientSecret) {
    console.error("GMAIL_CLIENT_ID and GMAIL_CLIENT_SECRET environment variables are required.");
    console.error("");
    console.error("Usage:");
    console.error(`  GMAIL_CLIENT_ID=<id> GMAIL_CLIENT_SECRET=<secret> npm run auth ${account}`);
    console.error("");
    console.error("Or set them in ~/.zshrc first, then run: npm run auth " + account);
    process.exit(1);
  }

  const oauth2Client = createOAuth2ClientWith(clientId, clientSecret);

  // Generate PKCE parameters (RFC 7636).
  const { codeVerifier, codeChallenge } = generatePKCE();

  const authUrl = oauth2Client.generateAuthUrl({
    access_type: "offline",
    scope: SCOPES,
    prompt: "consent",
    code_challenge: codeChallenge,
    code_challenge_method: CodeChallengeMethod.S256,
  });

  console.log(`\nAuthenticating account: "${account}"`);
  console.log(`Using OAuth client: ${clientId.substring(0, 20)}...`);
  console.log("Opening browser for Google authentication...");
  console.log("If it doesn't open automatically, visit:");
  console.log(authUrl);
  console.log();

  execFile("open", [authUrl]);

  return new Promise<void>((resolve, reject) => {
    const server = createServer(async (req, res) => {
      try {
        const url = new URL(req.url!, `http://127.0.0.1:${REDIRECT_PORT}`);
        const code = url.searchParams.get("code");
        const error = url.searchParams.get("error");

        if (error) {
          res.writeHead(400, { "Content-Type": "text/html" });
          res.end(`<h1>Authorization denied</h1><p>${error}</p>`);
          server.close();
          reject(new Error(`Authorization denied: ${error}`));
          return;
        }

        if (!code) {
          res.writeHead(400, { "Content-Type": "text/html" });
          res.end("<h1>No authorization code received</h1>");
          return;
        }

        const { tokens } = await oauth2Client.getToken({ code, codeVerifier });
        saveAccount(account, clientId, clientSecret, tokens);

        res.writeHead(200, { "Content-Type": "text/html" });
        res.end(
          `<h1>Gmail MCP authorized!</h1>` +
            `<p>Account: <strong>${account}</strong></p>` +
            `<p>You can close this tab and return to the terminal.</p>` +
            `<p>Scopes: <code>gmail.modify</code>, <code>gmail.compose</code></p>`
        );

        console.log(`Authorization successful! Token saved to ~/.gmail-mcp/${account}.json`);
        console.log("Scopes: gmail.modify, gmail.compose");

        server.close();
        resolve();
      } catch (err) {
        res.writeHead(500, { "Content-Type": "text/html" });
        res.end(`<h1>Error</h1><p>${err}</p>`);
        server.close();
        reject(err);
      }
    });

    server.listen(REDIRECT_PORT, "127.0.0.1", () => {
      console.log(`Listening for OAuth callback on ${REDIRECT_URI}...`);
    });
  });
}

// When run directly (npm run auth), execute the auth flow
const isDirectRun =
  process.argv[1]?.endsWith("auth.ts") ||
  process.argv[1]?.endsWith("auth.js");

if (isDirectRun) {
  // Filter out "--" that npm passes when using `npm run auth -- personal`
  const args = process.argv.slice(2).filter((a) => a !== "--");
  const account = args[0];
  if (!account) {
    console.error("Usage: npm run auth <account-name>");
    console.error("Examples:");
    console.error("  GMAIL_CLIENT_ID=<id> GMAIL_CLIENT_SECRET=<secret> npm run auth work");
    console.error("  GMAIL_CLIENT_ID=<id> GMAIL_CLIENT_SECRET=<secret> npm run auth personal");
    process.exit(1);
  }

  runAuthFlow(account)
    .then(() => process.exit(0))
    .catch((err) => {
      console.error("Auth failed:", err.message);
      process.exit(1);
    });
}
