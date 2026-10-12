export interface User {
  id: number;
  username: string;
  display_name: string;
  created_at: string;
  online?: boolean; // as of when the object was fetched; presence events keep it current
  avatar_url?: string; // uploaded picture under /media; empty = initials
  avatar_crop?: AvatarCrop; // the square shown in the circle; w === 0 means the whole picture, centred
  pronouns?: string;
}

// A square window on a picture: x/y top-left and w side as fractions of the
// width (the side as a fraction of the height is w * aspect).
export interface AvatarCrop {
  x: number;
  y: number;
  w: number;
  aspect: number;
}

// The signed-in user with private settings.
export interface Me extends User {
  invisible: boolean; // appear offline to everyone
  bio: string;
  must_change_password: boolean; // signed in with a temporary password: the app only shows the change-password screen
}

export type FriendStatus = 'self' | 'none' | 'friends' | 'outgoing' | 'incoming';

export interface UserProfile extends User {
  bio?: string;
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
  id: number;
  author: User;
  group_id: number | null;
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
  id: number;
  post_id: number;
  parent_id: number | null;
  edited_at: string | null;
  deleted: boolean; // body and author read [deleted]
  author: User;
  body: string;
  created_at: string;
}

export interface Group {
  id: number;
  name: string;
  description: string;
  owner_id: number;
  private: boolean; // invite only: hidden from Discover, the admin adds members
  created_at: string;
  member_count: number;
  my_role: 'admin' | 'member' | '';
}

export interface GroupDetail {
  group: Group;
  members: GroupMember[];
  call: number[]; // ids of the members in the group call right now
}

// What a call knows about its group.
export interface CallGroup {
  id: number;
  name: string;
}

export interface GroupMember extends User {
  role: 'admin' | 'member';
}

// Where a chat lives: with one friend, or in a group.
export type ChatTarget = { kind: 'user' | 'group'; id: number };

export interface Message {
  id: number;
  sender_id: number;
  recipient_id: number | null; // direct message: the other person
  group_id: number | null; // group chat
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
