import http from "k6/http";
import { check, sleep } from "k6";

const targetVUs = Number(__ENV.VUS || 20);
const baseURL = (__ENV.BASE_URL || "http://localhost:8080").replace(/\/$/, "");
const token = __ENV.TOKEN || "";

export const options = {
  stages: [
    { duration: "2m", target: targetVUs },
    { duration: "5m", target: targetVUs },
    { duration: "2m", target: 0 },
  ],
  thresholds: {
    http_req_failed: ["rate<0.02"],
    http_req_duration: ["p(95)<1500"],
  },
};

const notificationsEvery = Math.max(1, Number(__ENV.NOTIFICATIONS_EVERY || 1));
const dmThreadsEvery = Math.max(1, Number(__ENV.DM_THREADS_EVERY || 1));
let iteration = 0;

function isLocalBaseURL(raw) {
  try {
    const url = new URL(raw);
    return ["localhost", "127.0.0.1", "::1"].includes(url.hostname);
  } catch {
    return false;
  }
}

if (!isLocalBaseURL(baseURL) && __ENV.ALLOW_NON_LOCAL_LOAD_TEST !== "true") {
  throw new Error("Refusing non-local BASE_URL without ALLOW_NON_LOCAL_LOAD_TEST=true. Run load tests against staging or a production-like environment first.");
}

function shouldRunEvery(every) {
  return ((iteration + __VU) % every) === 0;
}

function headers() {
  const h = { Accept: "application/json" };
  if (token) {
    h.Authorization = `Bearer ${token}`;
  }
  return h;
}

export default function () {
  iteration += 1;
  const authed = Boolean(token);
  const feedPath = authed ? "/api/v1/posts/feed" : "/api/v1/public/posts/feed";
  const feed = http.get(`${baseURL}${feedPath}`, { headers: headers(), tags: { name: "feed" } });
  check(feed, {
    "feed status is 200": (r) => r.status === 200,
  });

  if (authed && shouldRunEvery(notificationsEvery)) {
    const notifications = http.get(`${baseURL}/api/v1/notifications`, {
      headers: headers(),
      tags: { name: "notifications" },
    });
    check(notifications, {
      "notifications status is 200": (r) => r.status === 200,
    });
  }

  if (authed && shouldRunEvery(dmThreadsEvery)) {
    const dmThreads = http.get(`${baseURL}/api/v1/dm/threads`, {
      headers: headers(),
      tags: { name: "dm_threads" },
    });
    check(dmThreads, {
      "dm threads status is 200": (r) => r.status === 200,
    });
  }

  sleep(Number(__ENV.SLEEP_SECONDS || 2));
}
