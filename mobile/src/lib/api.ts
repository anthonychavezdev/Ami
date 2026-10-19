import { API_URL } from './config';
import { uploadFile } from './upload';
import type {
  Attachment,
  ChatTarget,
  Comment,
  Conversation,
  Group,
  GroupDetail,
  Me,
  Message,
  Post,
  UploadProgress,
  User,
  UserProfile,
} from './types';
import { errorMessage } from './errors';

let authToken: string | null = null;
export function setAuthToken(token: string | null) {
  authToken = token;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
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

// Upload of a picked file: the file is the request body, streamed from disk
// (upload.ts on phones, upload.web.ts in the browser).
async function upload<T>(path: string, file: { uri: string; type: string }, onProgress?: (p: UploadProgress) => void): Promise<T> {
  const headers: Record<string, string> = {};
  if (authToken) headers.Authorization = `Bearer ${authToken}`;
  let res: { status: number; body: string };
  try {
    res = await uploadFile(API_URL + path, file, headers, onProgress);
  } catch (e) {
    throw new ApiError(0, `Cannot reach server at ${API_URL} (${errorMessage(e)})`);
  }
  let data: (T & { error?: string }) | null = null;
  try {
    data = res.body ? JSON.parse(res.body) : null;
  } catch {
    data = null;
  }
  if (res.status < 200 || res.status >= 300) throw new ApiError(res.status, data?.error ?? `HTTP ${res.status}`);
  return data as T;
}

// Absolute URL of an attachment (uploads are served by the API server).
export const mediaUrl = (a: Attachment) => (a.type === 'link' ? a.url : API_URL + a.url);

const get = <T>(path: string) => request<T>('GET', path);
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {});
const put = <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {});
const del = <T>(path: string, body?: unknown) => request<T>('DELETE', path, body);

// Direct messages live under /api/messages/{user}, group chats under /api/groups/{id}/messages.
const chatApi = (chat: ChatTarget) => (chat.kind === 'group' ? `/api/groups/${chat.id}/messages` : `/api/messages/${chat.id}`);

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
    post<{ token: string; user: Me }>('/api/register', { username, password, display_name }),
  login: (username: string, password: string) => post<{ token: string; user: Me }>('/api/login', { username, password }),
  logout: () => post<void>('/api/logout'),
  me: () => get<Me>('/api/me'),
  updateMe: (
    patch: Partial<
      Pick<
        Me,
        'username' | 'display_name' | 'bio' | 'pronouns' | 'interests' | 'links' | 'avatar_url' | 'avatar_crop' | 'cover_url' | 'cover_crop'
      >
    >,
  ) => put<Me>('/api/me', patch),
  setInvisible: (invisible: boolean) => put<{ invisible: boolean }>('/api/me/presence', { invisible }),
  changePassword: (current: string, next: string) => put<Me>('/api/me/password', { current, new: next }),

  // users
  searchUsers: (query: string) => get<UserProfile[]>(`/api/users${q({ q: query })}`),
  user: (id: string) => get<UserProfile>(`/api/users/${id}`),
  userPosts: (id: string, before?: string) => get<Post[]>(`/api/users/${id}/posts${q({ before })}`),

  // posts
  feed: (before?: string) => get<Post[]>(`/api/feed${q({ before })}`),
  createPost: (body: string, group_id?: string, attachments: Attachment[] = []) =>
    post<Post>('/api/posts', { body, group_id, attachments }),
  uploadMedia: (file: { uri: string; type: string }, onProgress?: (p: UploadProgress) => void) =>
    upload<Attachment>('/api/media', file, onProgress),
  post: (id: string) => get<Post>(`/api/posts/${id}`),
  deletePost: (id: string) => del<void>(`/api/posts/${id}`),
  updatePost: (id: string, body: string, attachments: Attachment[]) => put<Post>(`/api/posts/${id}`, { body, attachments }),
  comments: (postId: string) => get<Comment[]>(`/api/posts/${postId}/comments`),
  addComment: (postId: string, body: string, parentId?: string) =>
    post<Comment>(`/api/posts/${postId}/comments`, { body, parent_id: parentId ?? null }),
  updateComment: (id: string, body: string) => put<Comment>(`/api/comments/${id}`, { body }),
  deleteComment: (id: string) => del<void>(`/api/comments/${id}`),
  react: (postId: string, emoji: string) => put<Post>(`/api/posts/${postId}/reaction`, { emoji }),
  unreact: (postId: string) => del<Post>(`/api/posts/${postId}/reaction`),

  // friends
  friends: () => get<User[]>('/api/friends'),
  friendRequests: () => get<{ incoming: User[]; outgoing: User[] }>('/api/friends/requests'),
  sendFriendRequest: (id: string) => post<{ friend_status: string }>(`/api/friends/${id}/request`),
  acceptFriendRequest: (id: string) => post<{ friend_status: string }>(`/api/friends/${id}/accept`),
  removeFriend: (id: string) => del<{ friend_status: string }>(`/api/friends/${id}`),

  // groups
  myGroups: () => get<Group[]>('/api/groups?mine=1'),
  discoverGroups: (query: string) => get<Group[]>(`/api/groups${q({ q: query })}`),
  createGroup: (name: string, description: string, options: { private?: boolean; member_ids?: string[] } = {}) =>
    post<GroupDetail>('/api/groups', { name, description, ...options }),
  group: (id: string) => get<GroupDetail>(`/api/groups/${id}`),
  joinGroup: (id: string) => post<GroupDetail>(`/api/groups/${id}/join`),
  leaveGroup: (id: string) => post<GroupDetail>(`/api/groups/${id}/leave`),
  deleteGroup: (id: string) => del<void>(`/api/groups/${id}`),
  groupPosts: (id: string, before?: string) => get<Post[]>(`/api/groups/${id}/posts${q({ before })}`),
  addGroupMembers: (id: string, user_ids: string[]) => post<GroupDetail>(`/api/groups/${id}/members`, { user_ids }),
  removeGroupMember: (id: string, uid: string) => del<GroupDetail>(`/api/groups/${id}/members/${uid}`),

  // messages (direct and group chats)
  conversations: () => get<Conversation[]>('/api/conversations'),
  messages: (chat: ChatTarget, before?: string) => get<Message[]>(`${chatApi(chat)}${q({ before })}`),
  sendMessage: (chat: ChatTarget, body: string, attachments: Attachment[] = []) => post<Message>(chatApi(chat), { body, attachments }),
  updateMessage: (chat: ChatTarget, messageId: string, body: string) => put<Message>(`${chatApi(chat)}/${messageId}`, { body }),
  deleteMessage: (chat: ChatTarget, messageId: string) => del<void>(`${chatApi(chat)}/${messageId}`),
  markRead: (chat: ChatTarget) => post<{ unread: number }>(`${chatApi(chat)}/read`),
  setPushToken: (token: string) => put<void>('/api/push-token', { token }),
  deletePushToken: (token: string) => del<void>('/api/push-token', { token }),
};
