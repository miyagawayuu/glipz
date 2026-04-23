/** Data source for the right-sidebar operator announcements. Update this array when posting a new notice. */
export type OperatorAnnouncement = {
  id: string;
  title: string;
  body: string;
  /** Human-readable display date, such as YYYY-MM-DD. */
  date: string;
};

import { translateObject } from "../i18n";

export function getOperatorAnnouncements(): OperatorAnnouncement[] {
  return translateObject<OperatorAnnouncement[]>("data.operatorAnnouncements");
}
