export interface User {
  id: number;
  username: string;
  display_name: string;
  created_at: string;
  online?: boolean; // as of when the object was fetched; presence events keep it current
  avatar_url?: string; // uploaded picture under /media; empty = initials
  pronouns?: string;
}

// The signed-in user with private settings.
export interface Me extends User {
  invisible: boolean; // appear offline to everyone
  bio: string;
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
  created_at: string;
  member_count: number;
  my_role: 'admin' | 'member' | '';
}

export interface GroupMember extends User {
  role: 'admin' | 'member';
}

export interface Message {
  id: number;
  sender_id: number;
  recipient_id: number;
  body: string;
  attachments: Attachment[];
  created_at: string;
  edited_at: string | null;
}

export interface Conversation {
  user: User;
  last_message: Message;
  unread: number;
}
