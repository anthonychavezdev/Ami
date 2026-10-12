import { mediaUrl } from './api';
import type { Attachment } from './types';

// Browser: hand the file to the download manager.
export async function saveMedia(att: Attachment): Promise<string> {
  const res = await fetch(mediaUrl(att));
  if (!res.ok) throw new Error('Could not download the file');
  const url = URL.createObjectURL(await res.blob());
  const a = document.createElement('a');
  a.href = url;
  a.download = att.url.split('/').pop() || 'media';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
  return 'Download started';
}
