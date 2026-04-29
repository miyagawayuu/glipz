import { apiPublicGet } from "./api";

export type OperatorAnnouncement = {
  id: string;
  title: string;
  body: string;
  date: string;
};

export type PublicInstanceSettings = {
  registrations_enabled: boolean;
  minimum_registration_age: number;
  server_name: string;
  server_description: string;
  admin_name: string;
  admin_email: string;
  terms_url: string;
  privacy_policy_url: string;
  nsfw_guidelines_url: string;
  federation_policy_summary: string;
  operator_announcements: OperatorAnnouncement[];
};

export async function fetchPublicInstanceSettings(): Promise<PublicInstanceSettings> {
  return apiPublicGet<PublicInstanceSettings>("/api/v1/instance-settings");
}
