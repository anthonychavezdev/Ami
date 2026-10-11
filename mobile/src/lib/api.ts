import { File } from 'expo-file-system';

import { API_URL } from './config';
import type { Attachment, Comment, Conversation, Group, GroupMember, Message, Post, User, UserProfile } from './types';

let authToken: string | null = null;
export function setAuthToken(token: string | null) {
  authToken = token;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (authToken) headers.Authorization = `Bearer ${authToken}`;
  let res: Response;
  try {
    res = await fetch(API_URL + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  } catch {
    throw new ApiError(0, `Cannot reach server at ${API_URL}`);
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new ApiError(res.status, data?.error ?? res.statusText);
  return data as T;
}

// Multipart upload of a picked file. Expo's fetch wants a Blob, and a
// File from expo-file-system is one (its MIME type comes from the extension).
async function upload<T>(path: string, file: { uri: string; name: string; type: string }): Promise<T> {
  const form = new FormData();
  form.append('file', new File(file.uri), file.name);
  const headers: Record<string, string> = {};
  if (authToken) headers.Authorization = `Bearer ${authToken}`;
  let res: Response;
  try {
    res = await fetch(API_URL + path, { method: 'POST', headers, body: form });
  } catch (e) {
    throw new ApiError(0, `Cannot reach server at ${API_URL} (${e instanceof Error ? e.message : String(e)})`);
  }
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new ApiError(res.status, data?.error ?? res.statusText);
  return data as T;
}

// Absolute URL of an attachment (uploads are served by the API server).
export const mediaUrl = (a: Attachment) => (a.type === 'link' ? a.url : API_URL + a.url);

const get = <T>(path: string) => request<T>('GET', path);
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {});
const put = <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {});
const del = <T>(path: string) => request<T>('DELETE', path);

const q = (params: Record<string, string | number | undefined>) => {
  const s = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== '')
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
    .join('&');
  return s ? `?${s}` : '';
};

export const api = {
  // auth
  register: (username: string, password: string, display_name: string) =>
    post<{ token: string; user: User }>('/api/register', { username, password, display_name }),
  login: (username: string, password: string) => post<{ token: string; user: User }>('/api/login', { username, password }),
  logout: () => post<void>('/api/logout'),
  me: () => get<User>('/api/me'),

  // users
  searchUsers: (query: string) => get<UserProfile[]>(`/api/users${q({ q: query })}`),
  user: (id: number) => get<UserProfile>(`/api/users/${id}`),
  userPosts: (id: number, before?: number) => get<Post[]>(`/api/users/${id}/posts${q({ before })}`),

  // posts
  feed: (before?: number) => get<Post[]>(`/api/feed${q({ before })}`),
  createPost: (body: string, group_id?: number, attachments: Attachment[] = []) => post<Post>('/api/posts', { body, group_id, attachments }),
  uploadMedia: (file: { uri: string; name: string; type: string }) => upload<Attachment>('/api/media', file),
  post: (id: number) => get<Post>(`/api/posts/${id}`),
  deletePost: (id: number) => del<void>(`/api/posts/${id}`),
  comments: (postId: number) => get<Comment[]>(`/api/posts/${postId}/comments`),
  addComment: (postId: number, body: string) => post<Comment>(`/api/posts/${postId}/comments`, { body }),
  react: (postId: number, emoji: string) => put<Post>(`/api/posts/${postId}/reaction`, { emoji }),
  unreact: (postId: number) => del<Post>(`/api/posts/${postId}/reaction`),

  // friends
  friends: () => get<User[]>('/api/friends'),
  friendRequests: () => get<{ incoming: User[]; outgoing: User[] }>('/api/friends/requests'),
  sendFriendRequest: (id: number) => post<{ friend_status: string }>(`/api/friends/${id}/request`),
  acceptFriendRequest: (id: number) => post<{ friend_status: string }>(`/api/friends/${id}/accept`),
  removeFriend: (id: number) => del<{ friend_status: string }>(`/api/friends/${id}`),

  // groups
  myGroups: () => get<Group[]>('/api/groups?mine=1'),
  discoverGroups: (query: string) => get<Group[]>(`/api/groups${q({ q: query })}`),
  createGroup: (name: string, description: string) => post<{ group: Group; members: GroupMember[] }>('/api/groups', { name, description }),
  group: (id: number) => get<{ group: Group; members: GroupMember[] }>(`/api/groups/${id}`),
  joinGroup: (id: number) => post<{ group: Group; members: GroupMember[] }>(`/api/groups/${id}/join`),
  leaveGroup: (id: number) => post<{ group: Group; members: GroupMember[] }>(`/api/groups/${id}/leave`),
  deleteGroup: (id: number) => del<void>(`/api/groups/${id}`),
  groupPosts: (id: number, before?: number) => get<Post[]>(`/api/groups/${id}/posts${q({ before })}`),
  removeGroupMember: (id: number, uid: number) => del<{ group: Group; members: GroupMember[] }>(`/api/groups/${id}/members/${uid}`),

  // messages
  conversations: () => get<Conversation[]>('/api/conversations'),
  messages: (userId: number, before?: number) => get<Message[]>(`/api/messages/${userId}${q({ before })}`),
  sendMessage: (userId: number, body: string) => post<Message>(`/api/messages/${userId}`, { body }),
};
