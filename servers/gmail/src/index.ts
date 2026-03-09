#!/usr/bin/env node

import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { loadCredentials, listAccounts } from "./auth.js";
import { searchMessages, readMessage, readThread } from "./gmail.js";
import type { OAuth2Client } from "google-auth-library";

const server = new McpServer({
  name: "gmail-mcp",
  version: "0.2.0",
});

const authCache = new Map<string, OAuth2Client>();

function getAuth(account: string): OAuth2Client {
  const cached = authCache.get(account);
  if (cached) return cached;

  const client = loadCredentials(account);
  if (!client) {
    const available = listAccounts();
    throw new Error(
      `Account "${account}" not authenticated.\n` +
        `Run: cd ~/Personal/applications/gmail-mcp && npm run auth ${account}\n` +
        (available.length > 0
          ? `Available accounts: ${available.join(", ")}`
          : "No accounts authenticated yet.")
    );
  }

  authCache.set(account, client);
  return client;
}

// @ts-expect-error - known zod/MCP SDK type depth issue (same in things-mcp)
server.tool(
  "gmail_search",
  {
    account: z.string().describe("Account name to search (e.g., 'work' or 'personal')"),
    query: z.string().describe("Gmail search query (same syntax as Gmail search bar)"),
    maxResults: z
      .number()
      .optional()
      .describe("Maximum number of results to return (default 20, max 100)"),
  },
  async ({ account, query, maxResults }) => {
    const client = getAuth(account);
    const capped = Math.min(maxResults ?? 20, 100);

    const results = await searchMessages(client, query, capped);

    if (results.length === 0) {
      return {
        content: [{ type: "text", text: `No messages found in "${account}" for: ${query}` }],
      };
    }

    const formatted = results
      .map(
        (m, i) =>
          `${i + 1}. [${m.id}] ${m.date}\n   From: ${m.from}\n   To: ${m.to}\n   Subject: ${m.subject}\n   Snippet: ${m.snippet}\n   Thread: ${m.threadId}`
      )
      .join("\n\n");

    return {
      content: [
        {
          type: "text",
          text: `Found ${results.length} message(s) in "${account}" for: ${query}\n\n${formatted}`,
        },
      ],
    };
  }
);

server.tool(
  "gmail_read_message",
  {
    account: z.string().describe("Account name (e.g., 'work' or 'personal')"),
    messageId: z.string().describe("The message ID (from gmail_search results)"),
  },
  async ({ account, messageId }) => {
    const client = getAuth(account);
    const msg = await readMessage(client, messageId);

    const parts = [
      `Account: ${account}`,
      `From: ${msg.from}`,
      `To: ${msg.to}`,
      msg.cc ? `Cc: ${msg.cc}` : null,
      `Subject: ${msg.subject}`,
      `Date: ${msg.date}`,
      msg.attachments.length > 0
        ? `Attachments: ${msg.attachments.join(", ")}`
        : null,
      `\n---\n\n${msg.body}`,
    ]
      .filter(Boolean)
      .join("\n");

    return {
      content: [{ type: "text", text: parts }],
    };
  }
);

server.tool(
  "gmail_read_thread",
  {
    account: z.string().describe("Account name (e.g., 'work' or 'personal')"),
    threadId: z.string().describe("The thread ID (from gmail_search results)"),
  },
  async ({ account, threadId }) => {
    const client = getAuth(account);
    const thread = await readThread(client, threadId);

    if (thread.messages.length === 0) {
      return {
        content: [{ type: "text", text: `Thread ${threadId} has no messages.` }],
      };
    }

    const formatted = thread.messages
      .map((msg, i) => {
        const parts = [
          `--- Message ${i + 1} of ${thread.messages.length} [${msg.id}] ---`,
          `From: ${msg.from}`,
          `To: ${msg.to}`,
          msg.cc ? `Cc: ${msg.cc}` : null,
          `Subject: ${msg.subject}`,
          `Date: ${msg.date}`,
          msg.attachments.length > 0
            ? `Attachments: ${msg.attachments.join(", ")}`
            : null,
          `\n${msg.body}`,
        ]
          .filter(Boolean)
          .join("\n");
        return parts;
      })
      .join("\n\n");

    return {
      content: [
        {
          type: "text",
          text: `Thread ${threadId} in "${account}" (${thread.messages.length} messages):\n\n${formatted}`,
        },
      ],
    };
  }
);

server.tool(
  "gmail_accounts",
  {},
  async () => {
    const accounts = listAccounts();
    if (accounts.length === 0) {
      return {
        content: [
          {
            type: "text",
            text: "No accounts authenticated.\nRun: cd ~/Personal/applications/gmail-mcp && npm run auth <account-name>",
          },
        ],
      };
    }
    return {
      content: [
        { type: "text", text: `Authenticated accounts: ${accounts.join(", ")}` },
      ],
    };
  }
);

const transport = new StdioServerTransport();
await server.connect(transport);
