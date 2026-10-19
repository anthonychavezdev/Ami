import * as ImagePicker from 'expo-image-picker';
import { useState } from 'react';

import { api } from './api';
import { toast } from './toast';
import type { Attachment, UploadProgress } from './types';
import { errorMessage } from './errors';

const MAX_ATTACHMENTS = 10;

// The upload in flight, for a progress bar: what it is, how far it has got
// and how many picked files are queued behind it.
export type Upload = UploadProgress & { kind: 'picture' | 'video'; waiting: number };

// Attachments being put on a post or message: pick pictures and videos (each
// uploads as soon as it is chosen, one at a time), add links, remove any of them.
export function useAttachments(initial: Attachment[] = []) {
  const [attachments, setAttachments] = useState<Attachment[]>(initial);
  const [uploading, setUploading] = useState(0); // picked files not yet uploaded, the one in flight included
  const [upload, setUpload] = useState<Upload | null>(null);

  const pickMedia = async () => {
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ['images', 'videos'],
      allowsMultipleSelection: true,
      selectionLimit: Math.max(1, MAX_ATTACHMENTS - attachments.length),
      quality: 0.85,
    });
    if (result.canceled) return;
    setUploading((n) => n + result.assets.length);
    for (const [i, asset] of result.assets.entries()) {
      const kind = asset.type === 'video' ? 'video' : 'picture';
      const waiting = result.assets.length - i - 1;
      setUpload({ kind, waiting, sent: 0, total: 0 });
      try {
        const uploaded = await api.uploadMedia(uploadOf(asset), (p) => setUpload({ kind, waiting, ...p }));
        setAttachments((prev) => [...prev, uploaded]);
      } catch (e) {
        toast.error(`Upload failed: ${errorMessage(e)}`);
      } finally {
        setUploading((n) => n - 1);
      }
    }
    setUpload(null);
  };

  const addLink = (url: string) => setAttachments((prev) => [...prev, { type: 'link', url }]);
  const removeAt = (index: number) => setAttachments((prev) => prev.filter((_, i) => i !== index));
  const clear = () => setAttachments([]);

  return { attachments, uploading, upload, pickMedia, addLink, removeAt, clear };
}

// What to upload for a picked asset, with a sensible type for assets without one.
function uploadOf(asset: ImagePicker.ImagePickerAsset) {
  return { uri: asset.uri, type: asset.mimeType ?? (asset.type === 'video' ? 'video/mp4' : 'image/jpeg') };
}
