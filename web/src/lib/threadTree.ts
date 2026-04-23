import type { TimelinePost } from "../types/timeline";

export type ThreadNode = { post: TimelinePost; children: ThreadNode[] };

/** Builds a reply tree beneath a root post from flat rows carrying reply_to_post_id. */
export function buildReplyTree(rootId: string, flat: TimelinePost[]): ThreadNode[] {
  const ids = new Set<string>([rootId, ...flat.map((p) => p.id)]);
  const childrenMap = new Map<string, TimelinePost[]>();
  for (const p of flat) {
    const parent = p.reply_to_post_id;
    if (!parent || !ids.has(parent)) continue;
    const arr = childrenMap.get(parent);
    if (arr) arr.push(p);
    else childrenMap.set(parent, [p]);
  }
  for (const [, arr] of childrenMap) {
    arr.sort((a, b) => {
      const ta = a.visible_at || a.created_at || "";
      const tb = b.visible_at || b.created_at || "";
      if (ta !== tb) return ta.localeCompare(tb);
      return a.id.localeCompare(b.id);
    });
  }
  function nest(parentId: string): ThreadNode[] {
    return (childrenMap.get(parentId) ?? []).map((post) => ({
      post,
      children: nest(post.id),
    }));
  }
  return nest(rootId);
}

export function flatRepliesDFS(nodes: ThreadNode[]): TimelinePost[] {
  const out: TimelinePost[] = [];
  const walk = (ns: ThreadNode[]) => {
    for (const n of ns) {
      out.push(n.post);
      if (n.children.length) walk(n.children);
    }
  };
  walk(nodes);
  return out;
}

/** Computes left indentation in pixels from tree depth, treating direct children of the root as depth 1. */
export function depthIndentPxByPostId(nodes: ThreadNode[], unitPx = 12): Record<string, number> {
  const m: Record<string, number> = {};
  const walk = (ns: ThreadNode[], depth: number) => {
    for (const n of ns) {
      m[n.post.id] = depth * unitPx;
      if (n.children.length) walk(n.children, depth + 1);
    }
  };
  walk(nodes, 1);
  return m;
}
