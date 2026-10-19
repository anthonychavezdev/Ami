export interface User {
  id: string;
  username: string;
  display_name: string;
  created_at: string;
  online?: boolean; // as of when the object was fetched; presence events keep it current
  avatar_url?: string; // uploaded picture under /media; empty = initials
  avatar_crop?: Crop; // the square shown in the circle; w === 0 means the whole picture, centred
  pronouns?: string;
}

// A window on a picture: x/y top-left and w width as fractions of the
// picture's width, aspect the picture's width/height. The window's own shape
// is fixed (1 for the round profile picture, 3 for the cover), so its height
// as a fraction of the picture's height is w * aspect / ratio.
export interface Crop {
  x: number;
  y: number;
  w: number;
  aspect: number;
}

// One of a person's other places, shown on their profile.
export interface ProfileLink {
  label: string;
  url: string;
}

// The signed-in user with private settings.
export interface Me extends User {
  invisible: boolean; // appear offline to everyone
  bio: string; // Markdown
  interests: string[]; // tags; search finds people by them
  links: ProfileLink[];
  cover_url: string; // the wide picture across the top of the profile, under /media; empty = none
  cover_crop?: Crop; // its 3:1 window
  must_change_password: boolean; // signed in with a temporary password: the app only shows the change-password screen
}

export type FriendStatus = 'self' | 'none' | 'friends' | 'outgoing' | 'incoming';

export interface UserProfile extends User {
  bio?: string;
  interests?: string[];
  links?: ProfileLink[];
  cover_url?: string;
  cover_crop?: Crop;
  friend_status: FriendStatus;
  friend_count?: number;
  post_count?: number;
}

export type AttachmentType = 'image' | 'video' | 'link';

export interface Attachment {
  type: AttachmentType;
  url: string; // /media/<name> for uploads, absolute for links
  width?: number;
  height?: number;
}

export interface Post {
  id: string;
  author: User;
  group_id: string | null;
  group_name?: string;
  body: string;
  attachments: Attachment[];
  created_at: string;
  edited_at: string | null;
  comment_count: number;
  reactions: Record<string, number>;
  my_reaction: string;
}

export interface Comment {
  id: string;
  post_id: string;
  parent_id: string | null;
  edited_at: string | null;
  deleted: boolean; // body and author read [deleted]
  author: User;
  body: string;
  created_at: string;
}

export interface Group {
  id: string;
  name: string;
  description: string;
  owner_id: string;
  private: boolean; // invite only: hidden from Discover, the admin adds members
  created_at: string;
  member_count: number;
  my_role: 'admin' | 'member' | '';
}

export interface GroupDetail {
  group: Group;
  members: GroupMember[];
  call: string[]; // ids of the members in the group call right now
}

// What a call knows about its group.
export interface CallGroup {
  id: string;
  name: string;
}

export interface GroupMember extends User {
  role: 'admin' | 'member';
}

// Where a chat lives: with one friend, or in a group.
export type ChatTarget = { kind: 'user' | 'group'; id: string };

export interface Message {
  id: string;
  sender_id: string;
  recipient_id: string | null; // direct message: the other person
  group_id: string | null; // group chat
  sender?: User; // group chats name their author
  body: string;
  attachments: Attachment[];
  created_at: string;
  edited_at: string | null;
}

export interface Conversation {
  user?: User; // direct chat
  group?: Group; // group chat
  last_message: Message | null; // null: a group nobody has written in yet
  unread: number;
}

// How far an upload has got, in bytes, and how to follow or cancel one
// (upload.ts / upload.web.ts).
export type UploadProgress = { sent: number; total: number };
export type UploadOptions = { onProgress?: (progress: UploadProgress) => void; signal?: AbortSignal };
