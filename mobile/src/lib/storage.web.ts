// Browser build: localStorage, which is per origin and survives reloads.
export const storage = {
  get: async (key: string) => {
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  },
  set: async (key: string, value: string) => {
    localStorage.setItem(key, value);
  },
  remove: async (key: string) => {
    localStorage.removeItem(key);
  },
};
