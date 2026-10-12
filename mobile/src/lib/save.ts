import { File, Paths } from 'expo-file-system';
import * as MediaLibrary from 'expo-media-library/legacy';

import { mediaUrl } from './api';
import type { Attachment } from './types';

// Downloads a picture or video and adds it to the device's photo library.
// Resolves to a short confirmation for the viewer to show.
export async function saveMedia(att: Attachment): Promise<string> {
  const { status } = await MediaLibrary.requestPermissionsAsync(true);
  if (status !== 'granted') throw new Error('Allow access to your photo library to save');
  const dest = new File(Paths.cache, att.url.split('/').pop() || 'media');
  if (dest.exists) dest.delete();
  const file = await File.downloadFileAsync(mediaUrl(att), dest);
  await MediaLibrary.saveToLibraryAsync(file.uri);
  return att.type === 'video' ? 'Video saved to your library' : 'Picture saved to your library';
}
