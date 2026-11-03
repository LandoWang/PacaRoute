// Message to show for a caught value (API errors and thrown Errors).
export const errorMessage = (e: unknown) =>
  e instanceof Error ? e.message : String(e);
