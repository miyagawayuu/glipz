/**
 * Generates web/public/openapi.yaml from route inventory (keep in sync with backend/internal/httpserver/server.go).
 * Run: node scripts/build-openapi.mjs
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const outPath = path.join(__dirname, "..", "public", "openapi.yaml");

const infoDescription = `Glipz versioned REST API under this document's server base URL.

## Audience
BOT operators, integrations, internal scripts, and alternative clients. Anything not described here should be treated as undocumented and may change without notice.

## Base URL
Replace the example host with your real API origin. Paths in this document are relative to \`/api/v1\`. CORS follows the server allowlist.

## Federation
Instance-to-instance federation is documented separately (Federation protocol guidelines in the web app at \`/federation/guidelines\`). This API is mainly for same-instance user actions.

## Authentication
- **Login JWT** from \`POST /auth/login\` (plus MFA when enabled)—same format the web app stores.
- **Personal access tokens (PATs)** start with \`glpat_\` and are meant for server-side bots as raw Bearer tokens.
- **OAuth2**: \`client_credentials\` and \`authorization_code\`. Token endpoint: \`POST /oauth/token\` with \`application/x-www-form-urlencoded\`.

Always send \`Authorization: Bearer <token>\` for protected routes. Never put tokens in query strings.

For authorization code, open \`/developer/oauth/authorize\` on the **web app origin** (not necessarily the API host) with \`client_id\`, \`redirect_uri\`, optional \`state\`, then exchange the returned \`code\` at the token endpoint. The \`authorizationUrl\` in this document uses the same placeholder host as the API for tooling only—replace it with your real web origin when configuring clients.

## Conventions
JSON bodies use \`Content-Type: application/json\` with UTF-8. Media upload uses \`multipart/form-data\`.

Errors often return JSON with an \`error\` code (e.g. \`unauthorized\`, \`invalid_json\`). On rate limits you may receive \`429 Too Many Requests\`; honour \`Retry-After\` when present.

## SSE
Streams such as \`/posts/feed/stream\` may stay connected; use exponential backoff on reconnect.

## Disclaimer
Undocumented fields or behaviour may change without notice. Uses that violate the Terms, Privacy Policy, or law are prohibited.`;

/** @type {{ path: string, method: string, summary: string, tag: string, security: 'none' | 'optional' | 'bearer' | 'admin' }}[]} */
const routes = [
  { path: "/auth/register", method: "post", summary: "Start registration", tag: "Auth", security: "none" },
  { path: "/auth/handle-availability", method: "get", summary: "Check handle availability", tag: "Auth", security: "none" },
  { path: "/auth/register/verify", method: "post", summary: "Verify registration", tag: "Auth", security: "none" },
  { path: "/auth/login", method: "post", summary: "Login (JWT)", tag: "Auth", security: "none" },
  { path: "/auth/mfa/verify", method: "post", summary: "Complete MFA login", tag: "Auth", security: "none" },
  { path: "/oauth/token", method: "post", summary: "OAuth2 token endpoint", tag: "Auth", security: "none" },
  { path: "/custom-emojis", method: "get", summary: "List enabled custom emojis", tag: "Custom emojis", security: "none" },
  { path: "/public/posts/feed", method: "get", summary: "Public feed", tag: "Public", security: "none" },
  { path: "/public/posts/feed/stream", method: "get", summary: "Public feed SSE", tag: "Public", security: "none" },
  { path: "/public/federation/profile", method: "get", summary: "Remote profile by acct", tag: "Public federation", security: "none" },
  { path: "/public/federation/posts", method: "get", summary: "Remote actor posts", tag: "Public federation", security: "none" },
  { path: "/public/federation/incoming/{id}", method: "get", summary: "Public federated incoming post", tag: "Public federation", security: "none" },
  { path: "/public/federation/incoming/{id}/thread", method: "get", summary: "Public federated thread", tag: "Public federation", security: "none" },
  { path: "/media/object/{objectKey}", method: "get", summary: "Fetch media object by key", tag: "Media", security: "none" },
  { path: "/media/object/{objectKey}", method: "head", summary: "Media object metadata", tag: "Media", security: "none" },
  { path: "/link-preview", method: "get", summary: "Link preview", tag: "Public optional", security: "optional" },
  { path: "/users/by-handle/{handle}", method: "get", summary: "Profile by handle", tag: "Public optional", security: "optional" },
  { path: "/users/by-handle/{handle}/followers", method: "get", summary: "Followers by handle", tag: "Public optional", security: "optional" },
  { path: "/users/by-handle/{handle}/following", method: "get", summary: "Following by handle", tag: "Public optional", security: "optional" },
  { path: "/users/by-handle/{handle}/posts", method: "get", summary: "Posts by handle", tag: "Public optional", security: "optional" },
  { path: "/users/by-handle/{handle}/replies", method: "get", summary: "Replies by handle", tag: "Public optional", security: "optional" },
  { path: "/users/by-handle/{handle}/notes", method: "get", summary: "Notes list by handle", tag: "Public optional", security: "optional" },
  { path: "/users/by-handle/{handle}/post-media-tiles", method: "get", summary: "Post media tiles by handle", tag: "Public optional", security: "optional" },
  { path: "/posts/{postID}/feed-item", method: "get", summary: "Single feed item", tag: "Public optional", security: "optional" },
  { path: "/posts/{postID}/thread", method: "get", summary: "Post thread", tag: "Public optional", security: "optional" },
  { path: "/admin/federation/deliveries", method: "get", summary: "List federation deliveries (site admin)", tag: "Admin", security: "admin" },
  { path: "/admin/federation/delivery-counts", method: "get", summary: "Federation delivery counts", tag: "Admin", security: "admin" },
  { path: "/admin/federation/entitlements", method: "post", summary: "Issue federation entitlement JWT (site admin)", tag: "Admin", security: "admin" },
  { path: "/admin/federation/domain-blocks", method: "get", summary: "List domain blocks", tag: "Admin", security: "admin" },
  { path: "/admin/federation/domain-blocks", method: "post", summary: "Add domain block", tag: "Admin", security: "admin" },
  { path: "/admin/federation/domain-blocks", method: "delete", summary: "Remove domain block", tag: "Admin", security: "admin" },
  { path: "/admin/reports/posts", method: "get", summary: "List post reports", tag: "Admin", security: "admin" },
  { path: "/admin/reports/federated-posts", method: "get", summary: "List federated post reports", tag: "Admin", security: "admin" },
  { path: "/admin/reports/posts/{reportID}", method: "patch", summary: "Update post report status", tag: "Admin", security: "admin" },
  { path: "/admin/reports/federated-posts/{reportID}", method: "patch", summary: "Update federated report status", tag: "Admin", security: "admin" },
  { path: "/admin/users/by-handle/{handle}/badges", method: "get", summary: "Get user badges", tag: "Admin", security: "admin" },
  { path: "/admin/users/by-handle/{handle}/badges", method: "put", summary: "Set user badges", tag: "Admin", security: "admin" },
  { path: "/admin/custom-emojis/site", method: "get", summary: "List site custom emojis", tag: "Admin", security: "admin" },
  { path: "/admin/custom-emojis/site", method: "post", summary: "Create site custom emoji", tag: "Admin", security: "admin" },
  { path: "/admin/custom-emojis/site/{emojiID}", method: "patch", summary: "Patch site custom emoji", tag: "Admin", security: "admin" },
  { path: "/admin/custom-emojis/site/{emojiID}", method: "delete", summary: "Delete site custom emoji", tag: "Admin", security: "admin" },
  { path: "/admin/posts/{postID}/suspend-author", method: "post", summary: "Suspend post author", tag: "Admin", security: "admin" },
  { path: "/me", method: "get", summary: "Current user", tag: "Account", security: "bearer" },
  { path: "/me/custom-emojis", method: "get", summary: "List my custom emojis", tag: "Custom emojis", security: "bearer" },
  { path: "/me/custom-emojis", method: "post", summary: "Create my custom emoji", tag: "Custom emojis", security: "bearer" },
  { path: "/me/custom-emojis/{emojiID}", method: "patch", summary: "Patch my custom emoji", tag: "Custom emojis", security: "bearer" },
  { path: "/me/custom-emojis/{emojiID}", method: "delete", summary: "Delete my custom emoji", tag: "Custom emojis", security: "bearer" },
  { path: "/notifications", method: "get", summary: "List notifications", tag: "Notifications", security: "bearer" },
  { path: "/notifications/unread-count", method: "get", summary: "Unread notification count", tag: "Notifications", security: "bearer" },
  { path: "/notifications/read-all", method: "post", summary: "Mark all notifications read", tag: "Notifications", security: "bearer" },
  { path: "/notifications/stream", method: "get", summary: "Notification SSE", tag: "Notifications", security: "bearer" },
  { path: "/dm/identity", method: "get", summary: "DM identity", tag: "Direct messages", security: "bearer" },
  { path: "/dm/identity", method: "put", summary: "Update DM identity", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads", method: "get", summary: "List DM threads", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads", method: "post", summary: "Create DM thread", tag: "Direct messages", security: "bearer" },
  { path: "/dm/invite-peer", method: "post", summary: "Invite DM peer", tag: "Direct messages", security: "bearer" },
  { path: "/dm/unread-count", method: "get", summary: "DM unread count", tag: "Direct messages", security: "bearer" },
  { path: "/dm/stream", method: "get", summary: "DM SSE", tag: "Direct messages", security: "bearer" },
  { path: "/dm/upload", method: "post", summary: "DM upload", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}", method: "get", summary: "Get DM thread", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}/messages", method: "get", summary: "List DM messages", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}/call-history", method: "get", summary: "DM call history", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}/messages", method: "post", summary: "Create DM message", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}/read", method: "post", summary: "Mark DM thread read", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}/call-token", method: "post", summary: "Issue DM call token", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}/call-invite", method: "post", summary: "Invite DM call", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}/call-cancel", method: "post", summary: "Cancel DM call", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}/call-end", method: "post", summary: "End DM call", tag: "Direct messages", security: "bearer" },
  { path: "/dm/threads/{threadID}/call-missed", method: "post", summary: "Mark DM call missed", tag: "Direct messages", security: "bearer" },
  { path: "/auth/mfa/setup", method: "post", summary: "MFA setup", tag: "Auth", security: "bearer" },
  { path: "/auth/mfa/enable", method: "post", summary: "MFA enable", tag: "Auth", security: "bearer" },
  { path: "/media/presign", method: "post", summary: "Presign media upload", tag: "Media", security: "bearer" },
  { path: "/media/upload", method: "post", summary: "Upload media (multipart)", tag: "Media", security: "bearer" },
  { path: "/me/oauth-clients", method: "post", summary: "Create OAuth client", tag: "OAuth clients", security: "bearer" },
  { path: "/me/oauth-clients", method: "get", summary: "List OAuth clients", tag: "OAuth clients", security: "bearer" },
  { path: "/me/oauth-clients/{clientID}", method: "delete", summary: "Delete OAuth client", tag: "OAuth clients", security: "bearer" },
  { path: "/me/oauth-authorize", method: "post", summary: "OAuth authorize consent", tag: "OAuth clients", security: "bearer" },
  { path: "/me/personal-access-tokens", method: "post", summary: "Create PAT", tag: "Personal access tokens", security: "bearer" },
  { path: "/me/personal-access-tokens", method: "get", summary: "List PATs (prefixes)", tag: "Personal access tokens", security: "bearer" },
  { path: "/me/personal-access-tokens/{tokenID}", method: "delete", summary: "Revoke PAT", tag: "Personal access tokens", security: "bearer" },
  { path: "/me/dm-settings", method: "patch", summary: "Patch DM settings", tag: "Account", security: "bearer" },
  { path: "/me/web-push", method: "get", summary: "Web push config", tag: "Account", security: "bearer" },
  { path: "/me/web-push/subscription", method: "put", summary: "Put web push subscription", tag: "Account", security: "bearer" },
  { path: "/me/web-push/unsubscribe", method: "post", summary: "Unsubscribe web push", tag: "Account", security: "bearer" },
  { path: "/me/scheduled-posts", method: "get", summary: "List scheduled posts", tag: "Posts", security: "bearer" },
  { path: "/posts/feed", method: "get", summary: "Authenticated feed", tag: "Posts", security: "bearer" },
  { path: "/posts/bookmarks", method: "get", summary: "Bookmarks", tag: "Posts", security: "bearer" },
  { path: "/search", method: "get", summary: "Search", tag: "Search", security: "bearer" },
  { path: "/federation/remote-follow", method: "post", summary: "Remote follow", tag: "Federation", security: "bearer" },
  { path: "/federation/remote-follow", method: "get", summary: "Remote follow status", tag: "Federation", security: "bearer" },
  { path: "/federation/remote-follow", method: "delete", summary: "Remote follow delete", tag: "Federation", security: "bearer" },
  { path: "/federation/posts/{incomingID}/feed-item", method: "get", summary: "Federated incoming feed item", tag: "Federation", security: "bearer" },
  { path: "/federation/posts/{incomingID}/thread", method: "get", summary: "Federated incoming thread", tag: "Federation", security: "bearer" },
  { path: "/posts/feed/stream", method: "get", summary: "Feed SSE", tag: "Posts", security: "bearer" },
  { path: "/posts/{postID}/unlock", method: "post", summary: "Unlock post", tag: "Posts", security: "bearer" },
  { path: "/federation/posts/{incomingID}/unlock", method: "post", summary: "Unlock federated post", tag: "Federation", security: "bearer" },
  { path: "/posts/{postID}/reactions", method: "post", summary: "Add reaction", tag: "Posts", security: "bearer" },
  { path: "/posts/{postID}/reactions/{emoji}", method: "delete", summary: "Remove reaction", tag: "Posts", security: "bearer" },
  { path: "/posts/{postID}/poll/vote", method: "post", summary: "Vote on poll", tag: "Posts", security: "bearer" },
  { path: "/posts/{postID}/like", method: "post", summary: "Toggle like", tag: "Posts", security: "bearer" },
  { path: "/posts/{postID}/bookmark", method: "post", summary: "Toggle bookmark", tag: "Posts", security: "bearer" },
  { path: "/posts/{postID}/report", method: "post", summary: "Report post", tag: "Posts", security: "bearer" },
  { path: "/federation/posts/{incomingID}/poll/vote", method: "post", summary: "Vote on federated poll", tag: "Federation", security: "bearer" },
  { path: "/federation/posts/{incomingID}/like", method: "post", summary: "Toggle like on federated post", tag: "Federation", security: "bearer" },
  { path: "/federation/posts/{incomingID}/bookmark", method: "post", summary: "Toggle bookmark on federated post", tag: "Federation", security: "bearer" },
  { path: "/federation/posts/{incomingID}/report", method: "post", summary: "Report federated post", tag: "Federation", security: "bearer" },
  { path: "/federation/posts/{incomingID}/repost", method: "post", summary: "Toggle repost on federated post", tag: "Federation", security: "bearer" },
  { path: "/posts/{postID}/repost", method: "post", summary: "Toggle repost", tag: "Posts", security: "bearer" },
  { path: "/posts/{postID}", method: "patch", summary: "Update post", tag: "Posts", security: "bearer" },
  { path: "/posts/{postID}", method: "delete", summary: "Delete post", tag: "Posts", security: "bearer" },
  { path: "/posts", method: "post", summary: "Create post", tag: "Posts", security: "bearer" },
  { path: "/users/by-handle/{handle}/follow", method: "post", summary: "Toggle follow", tag: "Social", security: "bearer" },
  { path: "/me/profile", method: "patch", summary: "Patch profile", tag: "Account", security: "bearer" },
  { path: "/notes", method: "post", summary: "Create note", tag: "Notes", security: "bearer" },
  { path: "/notes/{noteID}", method: "get", summary: "Get note", tag: "Notes", security: "bearer" },
  { path: "/notes/{noteID}", method: "patch", summary: "Patch note", tag: "Notes", security: "bearer" },
  { path: "/notes/{noteID}", method: "delete", summary: "Delete note", tag: "Notes", security: "bearer" },
];

function yamlEscape(str) {
  if (/[\n"'#:[\]{}]/.test(str) || str.trim() !== str) {
    return JSON.stringify(str);
  }
  return str;
}

function securityYaml(kind) {
  if (kind === "none") return "      security: []\n";
  if (kind === "optional") return "      security:\n        - {}\n        - bearerAuth: []\n";
  if (kind === "bearer") return "      security:\n        - bearerAuth: []\n";
  if (kind === "admin") return "      security:\n        - bearerAuth: []\n";
  return "";
}

const tags = [
  "Auth",
  "Public",
  "Public federation",
  "Public optional",
  "Media",
  "Account",
  "Posts",
  "Federation",
  "Notifications",
  "Direct messages",
  "Search",
  "Social",
  "Notes",
  "OAuth clients",
  "Personal access tokens",
  "Custom emojis",
  "Admin",
];

let yaml = `openapi: 3.0.3
info:
  title: Glipz HTTP API
  version: "1.0.0"
  description: |
${infoDescription.split("\n").map((l) => `    ${l}`).join("\n")}
servers:
  - url: https://api.example.com/api/v1
    description: Replace the host with your API origin; all paths are under /api/v1.
tags:
`;

for (const t of tags) {
  yaml += `  - name: ${t}\n`;
}

yaml += `components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
      description: Login JWT, PAT (glpat_...), or OAuth2 access token.
    oauth2:
      type: oauth2
      flows:
        clientCredentials:
          tokenUrl: https://api.example.com/api/v1/oauth/token
          scopes: {}
        authorizationCode:
          authorizationUrl: https://api.example.com/developer/oauth/authorize
          tokenUrl: https://api.example.com/api/v1/oauth/token
          scopes: {}
  schemas:
`;

const postCreateYamlBlock = `
    PostCreate:
      type: object
      description: >
        Create a post. Handles text, images, video, polls, and scheduled posts.
        The server checks object_keys ownership under your uploads prefix and allows
        only a single media object when media_type is video.
      properties:
        caption:
          type: string
          description: Required if there is no media and no poll.
        media_type:
          type: string
          enum: [none, image, video]
          description: Recommended. If object_keys is empty, treated as none.
        object_keys:
          type: array
          items:
            type: string
          description: Up to 4. Required when posting media. Legacy single object_key is also accepted.
        object_key:
          type: string
          description: Legacy single object key (optional alternative to object_keys).
        is_nsfw:
          type: boolean
          description: Defaults to false.
        visibility:
          type: string
          description: Same set as the app (public, logged_in, followers, private).
        reply_to_post_id:
          type: string
          format: uuid
          description: Reply to a local post.
        reply_to_incoming_id:
          type: string
          format: uuid
          description: Reply to a federated incoming post; mutually exclusive with local reply fields.
        reply_to_object_url:
          type: string
          description: Reply targeting a remote object IRI.
        view_password:
          type: string
          description: Optional view password; use with scope and text range fields as supported by the server.
        visible_at:
          type: string
          format: date-time
          description: Scheduled publish time (RFC3339).
        poll:
          type: object
          description: Poll with options (2-4) and ends_at (RFC3339). Must end after visible_at when both are set.
          properties:
            options:
              type: array
              items:
                type: string
            ends_at:
              type: string
              format: date-time
`;

yaml += postCreateYamlBlock;
yaml += `
    UnlockPostRequest:
      type: object
      description: >
        Unlock a post. Use either a view password, or a short-lived entitlement JWT
        (JWS) issued by the origin instance for membership-gated content.
      properties:
        password:
          type: string
          description: View password when the post uses password protection.
        entitlement_jwt:
          type: string
          description: Short-lived JWS for membership unlock (EdDSA/Ed25519).

    AdminIssueFederationEntitlementRequest:
      type: object
      description: Issue an entitlement JWT (site-admin only; debugging / PoC).
      properties:
        post_id:
          type: string
          format: uuid
        viewer_acct:
          type: string
          description: Viewer acct, like user@example.com (must be the federated viewer identity).
      required: ["post_id", "viewer_acct"]
`;
yaml += `paths: {}
`;

/** @type {Record<string, Record<string, unknown>>} */
const paths = {};

for (const r of routes) {
  if (!paths[r.path]) paths[r.path] = {};
  const op = {
    tags: [r.tag],
    summary: r.summary,
    responses: {
      "200": { description: "Success (or streaming body for SSE)" },
      "401": { description: "Unauthorized" },
      "403": { description: "Forbidden" },
      "404": { description: "Not found" },
      "429": { description: "Too many requests" },
    },
  };
  if (r.security === "admin") {
    op.description = "Requires a valid access token and site-admin privileges.";
  }
  if (r.path === "/posts" && r.method === "post") {
    op.requestBody = {
      required: false,
      content: {
        "application/json": {
          schema: { $ref: "#/components/schemas/PostCreate" },
        },
      },
    };
  }
  if ((r.path === "/posts/{postID}/unlock" || r.path === "/federation/posts/{incomingID}/unlock") && r.method === "post") {
    op.requestBody = {
      required: true,
      content: {
        "application/json": {
          schema: { $ref: "#/components/schemas/UnlockPostRequest" },
        },
      },
    };
  }
  if (r.path === "/admin/federation/entitlements" && r.method === "post") {
    op.requestBody = {
      required: true,
      content: {
        "application/json": {
          schema: { $ref: "#/components/schemas/AdminIssueFederationEntitlementRequest" },
        },
      },
    };
  }
  if (r.path === "/media/upload" && r.method === "post") {
    op.requestBody = {
      content: {
        "multipart/form-data": {
          schema: {
            type: "object",
            properties: {
              file: { type: "string", format: "binary", description: "Upload file field name `file`." },
            },
            required: ["file"],
          },
        },
      },
    };
  }
  if (r.path === "/oauth/token" && r.method === "post") {
    op.requestBody = {
      required: true,
      content: {
        "application/x-www-form-urlencoded": {
          schema: {
            type: "object",
            required: ["grant_type", "client_id", "client_secret"],
            properties: {
              grant_type: { type: "string", enum: ["client_credentials", "authorization_code", "refresh_token"] },
              client_id: { type: "string" },
              client_secret: { type: "string" },
              code: { type: "string", description: "Authorization code (authorization_code grant)." },
              redirect_uri: { type: "string" },
              refresh_token: { type: "string" },
            },
          },
        },
      },
    };
  }
  paths[r.path][r.method] = op;
}

const mediaObjDesc =
  "Public media delivery. The server route matches a wildcard path after /media/object/; encode nested keys in the path or consult your deployment for how keys map to URLs.";
if (paths["/media/object/{objectKey}"]?.get) paths["/media/object/{objectKey}"].get.description = mediaObjDesc;
if (paths["/media/object/{objectKey}"]?.head) paths["/media/object/{objectKey}"].head.description = mediaObjDesc;

// Serialize paths to YAML (manual for control)
function dumpPaths(obj) {
  let y = "";
  for (const [p, methods] of Object.entries(obj)) {
    y += `  ${p}:\n`;
    for (const [m, op] of Object.entries(methods)) {
      y += `    ${m}:\n`;
      y += `      tags:\n        - ${op.tags[0]}\n`;
      y += `      summary: ${yamlEscape(op.summary)}\n`;
      if (op.description) y += `      description: ${yamlEscape(op.description)}\n`;
      const sec = routes.find((x) => x.path === p && x.method === m)?.security ?? "bearer";
      y += securityYaml(sec);
      if (op.requestBody) {
        y += `      requestBody:\n`;
        if (op.requestBody.required) y += `        required: true\n`;
        y += `        content:\n`;
        for (const [ct, body] of Object.entries(op.requestBody.content)) {
          y += `          ${ct}:\n`;
          if (body.schema?.$ref) {
            y += `            schema:\n              $ref: ${body.schema.$ref}\n`;
          } else {
            y += `            schema:\n              type: ${body.schema.type}\n`;
            if (body.schema.properties) {
              y += `              properties:\n`;
              for (const [pk, pv] of Object.entries(body.schema.properties)) {
                y += `                ${pk}:\n                  type: ${pv.type}\n`;
                if (pv.format) y += `                  format: ${pv.format}\n`;
                if (pv.enum) y += `                  enum: [${pv.enum.map((e) => `"${e}"`).join(", ")}]\n`;
                if (pv.description) y += `                  description: ${yamlEscape(pv.description)}\n`;
              }
            }
            if (body.schema.required) {
              y += `              required: [${body.schema.required.map((x) => `"${x}"`).join(", ")}]\n`;
            }
          }
        }
      }
      y += `      responses:\n`;
      for (const [code, resp] of Object.entries(op.responses)) {
        y += `        '${code}':\n          description: ${yamlEscape(resp.description)}\n`;
      }
    }
  }
  return y;
}

yaml = yaml.replace("paths: {}\n", "paths:\n" + dumpPaths(paths));

fs.mkdirSync(path.dirname(outPath), { recursive: true });
fs.writeFileSync(outPath, yaml, "utf8");
console.log("Wrote", outPath);
