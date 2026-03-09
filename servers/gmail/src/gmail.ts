import { google, gmail_v1 } from "googleapis";
import type { OAuth2Client } from "google-auth-library";

type GmailMessage = gmail_v1.Schema$Message;
type GmailMessagePart = gmail_v1.Schema$MessagePart;

function getGmailClient(auth: OAuth2Client) {
  return google.gmail({ version: "v1", auth });
}

function getHeader(message: GmailMessage, name: string): string {
  const header = message.payload?.headers?.find(
    (h) => h.name?.toLowerCase() === name.toLowerCase()
  );
  return header?.value || "";
}

function decodeBase64Url(data: string): string {
  return Buffer.from(data, "base64url").toString("utf8");
}

function extractTextBody(part: GmailMessagePart): string {
  // Direct text/plain body
  if (part.mimeType === "text/plain" && part.body?.data) {
    return decodeBase64Url(part.body.data);
  }

  // Recurse into multipart
  if (part.parts) {
    // Prefer text/plain over text/html
    for (const sub of part.parts) {
      if (sub.mimeType === "text/plain" && sub.body?.data) {
        return decodeBase64Url(sub.body.data);
      }
    }
    // Fall back to text/html, strip tags
    for (const sub of part.parts) {
      if (sub.mimeType === "text/html" && sub.body?.data) {
        const html = decodeBase64Url(sub.body.data);
        return stripHtml(html);
      }
    }
    // Recurse deeper
    for (const sub of part.parts) {
      const text = extractTextBody(sub);
      if (text) return text;
    }
  }

  // Single-part HTML fallback
  if (part.mimeType === "text/html" && part.body?.data) {
    return stripHtml(decodeBase64Url(part.body.data));
  }

  return "";
}

function stripHtml(html: string): string {
  return html
    .replace(/<br\s*\/?>/gi, "\n")
    .replace(/<\/p>/gi, "\n\n")
    .replace(/<\/div>/gi, "\n")
    .replace(/<[^>]+>/g, "")
    .replace(/&nbsp;/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function getAttachmentNames(message: GmailMessage): string[] {
  const names: string[] = [];
  function walk(part: GmailMessagePart) {
    if (part.filename && part.filename.length > 0) {
      names.push(part.filename);
    }
    if (part.parts) {
      part.parts.forEach(walk);
    }
  }
  if (message.payload) walk(message.payload);
  return names;
}

function formatMessage(message: GmailMessage) {
  const body = message.payload ? extractTextBody(message.payload) : "";
  const attachments = getAttachmentNames(message);

  return {
    id: message.id,
    threadId: message.threadId,
    from: getHeader(message, "From"),
    to: getHeader(message, "To"),
    cc: getHeader(message, "Cc"),
    subject: getHeader(message, "Subject"),
    date: getHeader(message, "Date"),
    body,
    snippet: message.snippet || "",
    attachments,
  };
}

export async function searchMessages(
  auth: OAuth2Client,
  query: string,
  maxResults: number = 20
) {
  const gmail = getGmailClient(auth);

  const listRes = await gmail.users.messages.list({
    userId: "me",
    q: query,
    maxResults,
  });

  const messageIds = listRes.data.messages || [];
  if (messageIds.length === 0) {
    return [];
  }

  // Batch-fetch metadata for each message
  const messages = await Promise.all(
    messageIds.map(async (m) => {
      const res = await gmail.users.messages.get({
        userId: "me",
        id: m.id!,
        format: "metadata",
        metadataHeaders: ["From", "To", "Subject", "Date"],
      });
      return {
        id: res.data.id,
        threadId: res.data.threadId,
        from: getHeader(res.data, "From"),
        to: getHeader(res.data, "To"),
        subject: getHeader(res.data, "Subject"),
        date: getHeader(res.data, "Date"),
        snippet: res.data.snippet || "",
      };
    })
  );

  return messages;
}

export async function readMessage(auth: OAuth2Client, messageId: string) {
  const gmail = getGmailClient(auth);

  const res = await gmail.users.messages.get({
    userId: "me",
    id: messageId,
    format: "full",
  });

  return formatMessage(res.data);
}

export async function readThread(auth: OAuth2Client, threadId: string) {
  const gmail = getGmailClient(auth);

  const res = await gmail.users.threads.get({
    userId: "me",
    id: threadId,
    format: "full",
  });

  const messages = (res.data.messages || []).map(formatMessage);
  return {
    id: res.data.id,
    messages,
  };
}
