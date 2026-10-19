// Browser build: the picker hands out blob: URLs, and the Blob behind one is
// backed by the file on disk, so the browser streams it as the request body.
export async function uploadFile(
  url: string,
  file: { uri: string; type: string },
  headers: Record<string, string>,
): Promise<{ status: number; body: string }> {
  const blob = await (await fetch(file.uri)).blob();
  const res = await fetch(url, { method: 'POST', headers: { ...headers, 'Content-Type': file.type }, body: blob });
  return { status: res.status, body: await res.text() };
}
