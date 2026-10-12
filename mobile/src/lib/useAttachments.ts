import * as ImagePicker from 'expo-image-picker';
import { useState } from 'react';

import { api } from './api';
import { toast } from './toast';
import type { Attachment } from './types';

const MAX_ATTACHMENTS = 10;

// Attachments being put on a post or message: pick pictures and videos (each
// uploads as soon as it is chosen), add links, remove any of them.
export function useAttachments(initial: Attachment[] = []) {
  const [attachments, setAttachments] = useState<Attachment[]>(initial);
  const [uploading, setUploading] = useState(0);

  const pickMedia = async () => {
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ['images', 'videos'],
      allowsMultipleSelection: true,
      selectionLimit: Math.max(1, MAX_ATTACHMENTS - attachments.length),
      quality: 0.85,
    });
    if (result.canceled) return;
    setUploading((n) => n + result.assets.length);
    for (const asset of result.assets) {
      try {
        const uploaded = await api.uploadMedia(uploadOf(asset));
        setAttachments((prev) => [...prev, uploaded]);
      } catch (e) {
        toast.error(`Upload failed: ${e instanceof Error ? e.message : String(e)}`);
      } finally {
        setUploading((n) => n - 1);
      }
    }
  };

  const addLink = (url: string) => setAttachments((prev) => [...prev, { type: 'link', url }]);
  const removeAt = (index: number) => setAttachments((prev) => prev.filter((_, i) => i !== index));
  const clear = () => setAttachments([]);

  return { attachments, uploading, pickMedia, addLink, removeAt, clear };
}

// What to upload for a picked asset, with sensible names for assets without one.
function uploadOf(asset: ImagePicker.ImagePickerAsset) {
  const video = asset.type === 'video';
  return { uri: asset.uri, name: asset.fileName ?? (video ? 'video.mp4' : 'photo.jpg'), type: asset.mimeType ?? (video ? 'video/mp4' : 'image/jpeg') };
}
