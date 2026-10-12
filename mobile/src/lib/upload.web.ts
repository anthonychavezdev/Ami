// Browser build: the picker hands out blob: and data: URLs.
export async function fileBlob(uri: string): Promise<Blob> {
  return (await fetch(uri)).blob();
}
