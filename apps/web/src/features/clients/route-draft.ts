import { type Route } from "../../api/types";
import { type ModelSelection } from "./model-routes";
const empty: ModelSelection = {
  source: "",
  models: [],
  defaultModel: "",
  fixed: false,
};
// An inline route draft remembers the saved routes it started from. `base`
// is undefined right after this card saved, so the next snapshot is adopted.
export type RouteDraft = {
  base: string | undefined;
  initial: ModelSelection;
  selection: ModelSelection;
};
export function routeDraft(
  saved: Route[],
  initial: ModelSelection | null,
): RouteDraft {
  const selection = initial || empty;
  return { base: JSON.stringify(saved), initial: selection, selection };
}
export const draftEdited = (draft: RouteDraft) =>
  JSON.stringify(draft.selection) !== JSON.stringify(draft.initial);
// Unrelated configuration changes keep the draft. When this Agent's saved
// routes change elsewhere, an untouched draft follows them; an edited draft
// is kept and reported as a conflict instead of being discarded or saved over
// the newer routes.
export function reconcileRouteDraft(
  draft: RouteDraft,
  saved: Route[],
  initial: ModelSelection | null,
  saving = false,
): { draft: RouteDraft; conflict: boolean } {
  if (saving || draft.base === JSON.stringify(saved))
    return { draft, conflict: false };
  if (draft.base === undefined || !draftEdited(draft))
    return { draft: routeDraft(saved, initial), conflict: false };
  return { draft, conflict: true };
}
