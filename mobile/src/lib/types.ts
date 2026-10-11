export interface User {
  id: number;
  username: string;
  display_name: string;
  created_at: string;
}

export interface UserProfile extends User {
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
