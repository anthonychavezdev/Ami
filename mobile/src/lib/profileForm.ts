import type { AvatarCrop, Me } from './types';

const USERNAME = /^[A-Za-z0-9_]{3,24}$/;

// What the edit-profile screen edits; the crop is null until the picture's size is known.
export type ProfileForm = { username: string; displayName: string; pronouns: string; bio: string; avatarUrl: string; crop: AvatarCrop | null };

// A changed profile field for PUT /api/me.
export type ProfilePatch = Partial<{ username: string; display_name: string; pronouns: string; bio: string; avatar_url: string; avatar_crop: AvatarCrop }>;

export const formFrom = (user: Me): ProfileForm => ({
  username: user.username,
  displayName: user.display_name,
  pronouns: user.pronouns ?? '',
  bio: user.bio,
  avatarUrl: user.avatar_url ?? '',
  crop: user.avatar_crop?.w ? user.avatar_crop : null,
});

const sameCrop = (a: AvatarCrop | null, b: AvatarCrop | undefined) => JSON.stringify(a) === JSON.stringify(b ?? null);

// Only the fields that differ from the saved profile.
export function profilePatch(form: ProfileForm, user: Me): ProfilePatch {
  const trimmed = { username: form.username.trim(), displayName: form.displayName.trim(), pronouns: form.pronouns.trim(), bio: form.bio.trim() };
  return {
    ...(trimmed.username !== user.username && { username: trimmed.username }),
    ...(trimmed.displayName !== user.display_name && { display_name: trimmed.displayName }),
    ...(trimmed.pronouns !== (user.pronouns ?? '') && { pronouns: trimmed.pronouns }),
    ...(trimmed.bio !== user.bio && { bio: trimmed.bio }),
    ...(form.avatarUrl !== (user.avatar_url ?? '') && { avatar_url: form.avatarUrl }),
    ...(form.avatarUrl && form.crop && !sameCrop(form.crop, user.avatar_crop) && { avatar_crop: form.crop }),
  };
}

export const usernameValid = (form: ProfileForm) => USERNAME.test(form.username.trim());

export const canSave = (form: ProfileForm, user: Me) => Object.keys(profilePatch(form, user)).length > 0 && usernameValid(form) && form.displayName.trim().length > 0;
