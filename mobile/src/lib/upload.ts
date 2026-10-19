import { File, UploadType } from 'expo-file-system';

import type { UploadProgress } from './types';

// Sends a picked file as the request body, streamed from disk by the OS.
// Expo's fetch would first read the whole file into memory, and uploads can
// be 20 GB. Progress arrives from the OS at most ten times a second.
export function uploadFile(
  url: string,
  file: { uri: string; type: string },
  headers: Record<string, string>,
  onProgress?: (progress: UploadProgress) => void,
): Promise<{ status: number; body: string }> {
  return new File(file.uri).upload(url, {
    httpMethod: 'POST',
    uploadType: UploadType.BINARY_CONTENT,
    headers: { ...headers, 'Content-Type': file.type },
    onProgress: onProgress && (({ bytesSent, totalBytes }) => onProgress({ sent: bytesSent, total: totalBytes })),
  });
}
