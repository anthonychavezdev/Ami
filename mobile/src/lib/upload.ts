import { File } from 'expo-file-system';

// A picked file as a Blob for FormData: Expo's fetch wants one and File is
// one (its MIME type comes from the extension).
export async function fileBlob(uri: string): Promise<Blob> {
  return new File(uri);
}
