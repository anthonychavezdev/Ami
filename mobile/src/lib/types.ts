export interface User {
  id: number;
  username: string;
  display_name: string;
  created_at: string;
}

export type FriendStatus = 'self' | 'none' | 'friends' | 'outgoing' | 'incoming';

export interface UserProfile extends User {
  friend_status: FriendStatus;
  friend_count?: number;
  post_count?: number;
}

export interface Post {
  id: number;
  author: User;
  body: string;
  created_at: string;
  comment_count: number;
  reactions: Record<string, number>;
  my_reaction: string;
}

export interface Comment {
  id: number;
  post_id: number;
  author: User;
  body: string;
  created_at: string;
}
