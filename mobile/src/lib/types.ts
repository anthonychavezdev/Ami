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
}
