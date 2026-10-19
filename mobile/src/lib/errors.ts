// The message of anything thrown, for toasts and banners.
export const errorMessage = (e: unknown) => (e instanceof Error ? e.message : String(e));

// A cancelled upload rejects with this (the name the platform APIs use too),
// so callers can tell it from a failure and stay quiet.
export function abortError(): Error {
  const e = new Error('Cancelled');
  e.name = 'AbortError';
  return e;
}
export const isAbort = (e: unknown) => e instanceof Error && e.name === 'AbortError';
