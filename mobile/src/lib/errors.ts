// The message of anything thrown, for toasts and banners.
export const errorMessage = (e: unknown) => (e instanceof Error ? e.message : String(e));
