/** Poll attached to a post, matching the API poll payload. */
export type TimelinePoll = {
  ends_at: string;
  closed: boolean;
  options: { id: string; label: string; votes: number }[];
  my_option_id?: string;
  total_votes: number;
};

export type ViewPasswordTextRange = {
  start: number;
  end: number;
};

export type TimelineReaction = {
  emoji: string;
  count: number;
  reacted_by_me: boolean;
};

/** One timeline entry shared by feed and user-page views. */
export type TimelinePost = {
  id: string;
  user_email: string;
  user_handle: string;
  /** Display name resolved by the server, falling back to the email-derived name when unset. */
  user_display_name?: string;
  user_badges?: string[];
  /** Public profile image URL, empty when not configured. */
  user_avatar_url?: string;
  caption: string;
  media_type: string;
  media_urls: string[];
  visibility?: "public" | "logged_in" | "followers" | "private";
  /** NSFW content. Media is shown only after age-gate confirmation. */
  is_nsfw?: boolean;
  /** Whether a view password is configured. */
  has_view_password?: boolean;
  /** Whether the post is membership-locked (federated/remote gating). */
  has_membership_lock?: boolean;
  /** Bitmask for protected targets: text, media, or all content. */
  view_password_scope?: number;
  /** Protected text ranges within the caption body. */
  view_password_text_ranges?: ViewPasswordTextRange[];
  /** Caption or media is still hidden because the post has not been unlocked. */
  content_locked?: boolean;
  /** Text content is still locked. */
  text_locked?: boolean;
  /** Media content is still locked. */
  media_locked?: boolean;
  created_at?: string;
  /** Publish time shown on the timeline. Scheduled posts enter feeds once this time is reached. */
  visible_at?: string;
  poll?: TimelinePoll | null;
  reactions: TimelineReaction[];
  reply_count: number;
  like_count: number;
  repost_count: number;
  liked_by_me: boolean;
  reposted_by_me: boolean;
  bookmarked_by_me?: boolean;
  /** Parent post ID, present only on reply rows returned by thread APIs. */
  reply_to_post_id?: string;
  /** Federated parent URL used when the parent has no local post ID. */
  reply_to_object_url?: string;
  /** Unique timeline-row key, even when repost rows share the original post ID. */
  feed_entry_id?: string;
  /** Remote post received through the federated timeline. */
  is_federated?: boolean;
  /** Whether the inbound event came from an Announce or boost. */
  federated_boost?: boolean;
  /** Original remote post URL. */
  remote_object_url?: string;
  /** Remote author profile (Actor) URL. */
  remote_actor_url?: string;
  /** When present, this row is a repost and the top-level fields describe the embedded original post. */
  repost?: {
    user_id: string;
    user_email: string;
    user_handle: string;
    user_display_name?: string;
    user_badges?: string[];
    user_avatar_url?: string;
    reposted_at: string;
    /** Optional comment added when reposting. */
    comment?: string;
  };
};
