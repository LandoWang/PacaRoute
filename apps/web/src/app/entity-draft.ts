import { useState, type SetStateAction } from "react";
// An unsaved edit of one saved entity. It remembers the saved value it
// started from; `base` is undefined right after the owner saved, so the next
// snapshot is adopted.
export type EntityDraft<T> = {
  base: string | undefined;
  initial: T;
  value: T;
};
export function entityDraft<T>(saved: unknown, initial: T): EntityDraft<T> {
  return { base: JSON.stringify(saved), initial, value: initial };
}
export const draftEdited = <T>(draft: EntityDraft<T>) =>
  JSON.stringify(draft.value) !== JSON.stringify(draft.initial);
// Unrelated configuration changes keep the draft. When the saved entity
// changes elsewhere, an untouched draft follows it; an edited draft is kept
// and reported as a conflict instead of being discarded or saved over the
// newer value.
export function reconcileDraft<T>(
  draft: EntityDraft<T>,
  saved: unknown,
  initial: () => T,
  saving = false,
): { draft: EntityDraft<T>; conflict: boolean } {
  if (saving || draft.base === JSON.stringify(saved))
    return { draft, conflict: false };
  if (draft.base === undefined || !draftEdited(draft))
    return { draft: entityDraft(saved, initial()), conflict: false };
  return { draft, conflict: true };
}
// Inline draft for one saved entity. `initial` derives the editable value
// from `saved`; pass `saving` while the owner's save is in flight.
export function useEntityDraft<T>(
  saved: unknown,
  initial: () => T,
  saving: boolean,
) {
  const [stored, setStored] = useState(() => entityDraft(saved, initial()));
  const { draft, conflict } = reconcileDraft(stored, saved, initial, saving);
  if (draft !== stored) setStored(draft);
  return {
    value: draft.value,
    conflict,
    set: (next: SetStateAction<T>) =>
      setStored((d) => ({
        ...d,
        value:
          typeof next === "function" ? (next as (v: T) => T)(d.value) : next,
      })),
    // Explicitly drop the edit and load the saved value.
    reset: () => setStored(entityDraft(saved, initial())),
    // Adopt the next snapshot as the saved value after a successful save.
    markSaved: () => setStored((d) => ({ ...d, base: undefined })),
  };
}
