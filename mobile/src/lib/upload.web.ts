import type { UploadProgress } from './types';

// Browser build: the picker hands out blob: URLs, and the Blob behind one is
// backed by the file on disk, so the browser streams it as the request body.
// XMLHttpRequest rather than fetch, which cannot report upload progress.
export async function uploadFile(
  url: string,
  file: { uri: string; type: string },
  headers: Record<string, string>,
  onProgress?: (progress: UploadProgress) => void,
): Promise<{ status: number; body: string }> {
  const blob = await (await fetch(file.uri)).blob();
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', url);
    for (const [name, value] of Object.entries({ ...headers, 'Content-Type': file.type })) xhr.setRequestHeader(name, value);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.({ sent: e.loaded, total: e.total });
    };
    xhr.onload = () => resolve({ status: xhr.status, body: xhr.responseText });
    xhr.onerror = () => reject(new Error('network request failed'));
    xhr.send(blob);
  });
}
