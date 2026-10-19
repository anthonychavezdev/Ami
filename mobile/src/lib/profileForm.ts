import { isHttpUrl } from './markdownEdit';
import type { Crop, Me, ProfileLink } from './types';

const USERNAME = /^[A-Za-z0-9_]{3,24}$/;

export const MAX_INTERESTS = 20;
export const MAX_LINKS = 10;

// What the profile editor edits; a crop is null until its picture's size is known.
export type ProfileForm = {
  username: string;
  displayName: string;
  pronouns: string;
  bio: string;
  interests: string[];
  links: ProfileLink[];
  avatarUrl: string;
  crop: Crop | null;
  cover: string;
  coverCrop: Crop | null;
};

// A changed profile field for PUT /api/me.
export type ProfilePatch = Partial<{
  username: string;
  display_name: string;
  pronouns: string;
  bio: string;
  interests: string[];
  links: ProfileLink[];
  avatar_url: string;
  avatar_crop: Crop;
  cover_url: string;
  cover_crop: Crop;
}>;

export const formFrom = (user: Me): ProfileForm => ({
  username: user.username,
  displayName: user.display_name,
  pronouns: user.pronouns ?? '',
  bio: user.bio,
  interests: user.interests ?? [],
  links: user.links ?? [],
  avatarUrl: user.avatar_url ?? '',
  crop: user.avatar_crop?.w ? user.avatar_crop : null,
  cover: user.cover_url ?? '',
  coverCrop: user.cover_crop?.w ? user.cover_crop : null,
});

const same = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null);

const trimmedLinks = (links: ProfileLink[]) => links.map((l) => ({ label: l.label.trim(), url: l.url.trim() }));

// Only the fields that differ from the saved profile.
export function profilePatch(form: ProfileForm, user: Me): ProfilePatch {
  const trimmed = {
    username: form.username.trim(),
    displayName: form.displayName.trim(),
    pronouns: form.pronouns.trim(),
    bio: form.bio.trim(),
  };
  const links = trimmedLinks(form.links);
  return {
    ...(trimmed.username !== user.username && { username: trimmed.username }),
    ...(trimmed.displayName !== user.display_name && { display_name: trimmed.displayName }),
    ...(trimmed.pronouns !== (user.pronouns ?? '') && { pronouns: trimmed.pronouns }),
    ...(trimmed.bio !== user.bio && { bio: trimmed.bio }),
    ...(!same(form.interests, user.interests ?? []) && { interests: form.interests }),
    ...(!same(links, user.links ?? []) && { links }),
    ...(form.avatarUrl !== (user.avatar_url ?? '') && { avatar_url: form.avatarUrl }),
    ...(form.avatarUrl && form.crop && !same(form.crop, user.avatar_crop) && { avatar_crop: form.crop }),
    ...(form.cover !== (user.cover_url ?? '') && { cover_url: form.cover }),
    ...(form.cover && form.coverCrop && !same(form.coverCrop, user.cover_crop) && { cover_crop: form.coverCrop }),
  };
}

export const usernameValid = (form: ProfileForm) => USERNAME.test(form.username.trim());

// Every link has an http(s) URL (a label is optional).
export const linksValid = (form: ProfileForm) => form.links.every((l) => isHttpUrl(l.url.trim()));

// Tags without blanks or repeats (ignoring case), in the order given.
export const withInterests = (current: string[], added: string[]) =>
  [...current, ...added.map((t) => t.trim()).filter(Boolean)]
    .filter((t, i, all) => all.findIndex((x) => x.toLowerCase() === t.toLowerCase()) === i)
    .slice(0, MAX_INTERESTS);

export const canSave = (form: ProfileForm, user: Me) =>
  Object.keys(profilePatch(form, user)).length > 0 && usernameValid(form) && form.displayName.trim().length > 0 && linksValid(form);
