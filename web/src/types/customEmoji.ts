export type CustomEmoji = {
  id: string;
  shortcode: string;
  shortcode_name: string;
  owner_handle?: string;
  domain?: string;
  image_url: string;
  is_enabled: boolean;
  scope: "site" | "user" | "remote";
  created_at: string;
  updated_at: string;
};
