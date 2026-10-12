import * as SecureStore from 'expo-secure-store';

// Small key-value store for the session token and preferences (Keychain / Keystore).
export const storage = {
  get: (key: string) => SecureStore.getItemAsync(key),
  set: (key: string, value: string) => SecureStore.setItemAsync(key, value),
  remove: (key: string) => SecureStore.deleteItemAsync(key),
};
