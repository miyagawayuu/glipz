/**
 * Curl / URL snippets for ApiDeveloperView.
 * Not in vue-i18n: `$` and `{` in shell/JSON are parsed as message-syntax tokens and break the compiler.
 */

export function formatApiDeveloperClientCredentialsCurl(base: string): string {
  return `curl -sS -X POST "${base}/api/v1/oauth/token" \\
  -H "Content-Type: application/x-www-form-urlencoded" \\
  --data-urlencode "grant_type=client_credentials" \\
  --data-urlencode "client_id=YOUR_CLIENT_UUID" \\
  --data-urlencode "client_secret=YOUR_SECRET"`;
}

export function formatApiDeveloperAuthorizationCodeCurl(base: string): string {
  return `curl -sS -X POST "${base}/api/v1/oauth/token" \\
  -H "Content-Type: application/x-www-form-urlencoded" \\
  --data-urlencode "grant_type=authorization_code" \\
  --data-urlencode "client_id=CLIENT_UUID" \\
  --data-urlencode "client_secret=SECRET" \\
  --data-urlencode "code=CODE_FROM_REDIRECT" \\
  --data-urlencode "redirect_uri=https://same-as-authorize"`;
}

export function formatApiDeveloperMediaUploadCurl(base: string): string {
  return `curl -sS -X POST "${base}/api/v1/media/upload" \\
  -H "Authorization: Bearer $TOKEN" \\
  -F "file=@./photo.jpg"`;
}

export function formatApiDeveloperAuthorizeUrlSample(base: string): string {
  return `${base}/developer/oauth/authorize?response_type=code&client_id=CLIENT_UUID&redirect_uri=https%3A%2F%2F…&state=RANDOM`;
}

export function formatApiDeveloperPostsExampleCurl(base: string): string {
  const createBody = JSON.stringify({
    caption: "hello from bot",
    media_type: "none",
    object_keys: [],
    is_nsfw: false,
  });
  const patchBody = JSON.stringify({ caption: "updated", is_nsfw: false });

  return `# Create post (no images)
curl -sS -X POST "${base}/api/v1/posts" \\
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \\
  -d '${createBody}'

# Edit
curl -sS -X PATCH "${base}/api/v1/posts/POST_UUID" \\
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \\
  -d '${patchBody}'

# Delete
curl -sS -X DELETE "${base}/api/v1/posts/POST_UUID" \\
  -H "Authorization: Bearer $TOKEN"`;
}
