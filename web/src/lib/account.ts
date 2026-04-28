import { api } from "./api";

export function accountDeletionInputReady(password: string, confirm: string): boolean {
  return password.length > 0 && confirm === "DELETE";
}

export async function deleteAccount(input: { password: string; confirm: string }): Promise<void> {
  await api<{ ok: true; status: "completed" }>("/api/v1/me/account", {
    method: "DELETE",
    json: input,
  });
}
