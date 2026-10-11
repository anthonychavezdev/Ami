import * as ImagePicker from 'expo-image-picker';
import React, { useRef, useState } from 'react';
import { ActivityIndicator, Alert, Image, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';

import { Button, Input } from './ui';
import { api, mediaUrl } from '@/lib/api';
import { useStyles, useTheme } from '@/lib/theme';
import type { Attachment } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

type Selection = { start: number; end: number };

// Generic "write something and submit" box used for posts, comments and chat.
// `rich` adds a Markdown toolbar plus picture, video and link attachments.
export function Composer({
  placeholder,
  buttonTitle = 'Post',
  onSubmit,
  multiline = true,
  compact = false,
  rich = false,
}: {
  placeholder: string;
  buttonTitle?: string;
  onSubmit: (text: string, attachments: Attachment[]) => Promise<void>;
  multiline?: boolean;
  compact?: boolean;
  rich?: boolean;
}) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const input = useRef<TextInput>(null);
  const [text, setText] = useState('');
  const [selection, setSelection] = useState<Selection>({ start: 0, end: 0 });
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [uploading, setUploading] = useState(0);
  const [linkDraft, setLinkDraft] = useState<string | null>(null); // null = link field hidden
  const [busy, setBusy] = useState(false);

  const canSubmit = (text.trim().length > 0 || attachments.length > 0) && uploading === 0 && !busy;

  const submit = async () => {
    if (!canSubmit) return;
    setBusy(true);
    try {
      await onSubmit(text.trim(), attachments);
      setText('');
      setAttachments([]);
      setLinkDraft(null);
    } catch (e) {
      Alert.alert('Could not send', e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  // Wrap the selection (or a placeholder) in Markdown markers and keep it selected.
  const wrap = (before: string, after: string, fallback: string) => {
    const { start, end } = selection;
    const inner = text.slice(start, end) || fallback;
    setText(text.slice(0, start) + before + inner + after + text.slice(end));
    setSelection({ start: start + before.length, end: start + before.length + inner.length });
    input.current?.focus();
  };
  // Prefix the current line (heading, quote, list item).
  const prefixLine = (prefix: string) => {
    const lineStart = text.lastIndexOf('\n', selection.start - 1) + 1;
    setText(text.slice(0, lineStart) + prefix + text.slice(lineStart));
    const caret = selection.end + prefix.length;
    setSelection({ start: caret, end: caret });
    input.current?.focus();
  };

  const pickMedia = async () => {
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ['images', 'videos'],
      allowsMultipleSelection: true,
      selectionLimit: Math.max(1, 10 - attachments.length),
      quality: 0.85,
    });
    if (result.canceled) return;
    setUploading((n) => n + result.assets.length);
    for (const asset of result.assets) {
      try {
        const video = asset.type === 'video';
        const att = await api.uploadMedia({
          uri: asset.uri,
          name: asset.fileName ?? (video ? 'video.mp4' : 'photo.jpg'),
          type: asset.mimeType ?? (video ? 'video/mp4' : 'image/jpeg'),
        });
        setAttachments((prev) => [...prev, att]);
      } catch (e) {
        Alert.alert('Upload failed', e instanceof Error ? e.message : String(e));
      } finally {
        setUploading((n) => n - 1);
      }
    }
  };

  const addLink = () => {
    const url = (linkDraft ?? '').trim();
    if (!/^https?:\/\/\S+$/i.test(url)) {
      Alert.alert('Invalid link', 'Links must start with http:// or https://');
      return;
    }
    setAttachments((prev) => [...prev, { type: 'link', url }]);
    setLinkDraft(null);
  };

  return (
    <View style={[styles.wrap, compact ? styles.row : null]}>
      {rich && (
        <View style={styles.toolbar}>
          <Tool label="B" bold onPress={() => wrap('**', '**', 'bold')} />
          <Tool label="I" italic onPress={() => wrap('_', '_', 'italic')} />
          <Tool label="H" onPress={() => prefixLine('# ')} />
          <Tool label="❝" onPress={() => prefixLine('> ')} />
          <Tool label="<>" onPress={() => wrap('`', '`', 'code')} />
          <Tool label="•" onPress={() => prefixLine('- ')} />
          <Tool label="🔗" onPress={() => wrap('[', '](https://)', 'link text')} />
          <View style={{ flex: 1 }} />
          <Tool label="🖼" onPress={() => void pickMedia()} />
          <Tool label="🌐" onPress={() => setLinkDraft((d) => (d === null ? '' : null))} />
        </View>
      )}
      <Input
        ref={input}
        value={text}
        onChangeText={setText}
        placeholder={placeholder}
        multiline={multiline}
        style={[compact ? styles.inputRow : styles.input, rich && styles.inputRich]}
        onSubmitEditing={multiline ? undefined : submit}
        returnKeyType={multiline ? 'default' : 'send'}
        selection={rich ? selection : undefined}
        onSelectionChange={rich ? (e) => setSelection(e.nativeEvent.selection) : undefined}
      />
      {rich && linkDraft !== null && (
        <View style={styles.linkRow}>
          <Input placeholder="https://…" autoCapitalize="none" autoCorrect={false} keyboardType="url" value={linkDraft} onChangeText={setLinkDraft} onSubmitEditing={addLink} style={{ flex: 1 }} />
          <Button title="Add link" small onPress={addLink} />
        </View>
      )}
      {rich && (attachments.length > 0 || uploading > 0) && (
        <View style={styles.chips}>
          {attachments.map((a, i) => (
            <View key={`${a.url}-${i}`} style={styles.chip}>
              {a.type === 'image' ? <Image source={{ uri: mediaUrl(a) }} style={styles.thumb} /> : <Text style={styles.chipIcon}>{a.type === 'video' ? '🎬' : '🔗'}</Text>}
              <Text numberOfLines={1} style={styles.chipText}>
                {a.type === 'link' ? a.url.replace(/^https?:\/\//, '') : a.type}
              </Text>
              <Pressable onPress={() => setAttachments((prev) => prev.filter((_, j) => j !== i))} hitSlop={8}>
                <Text style={styles.chipRemove}>✕</Text>
              </Pressable>
            </View>
          ))}
          {uploading > 0 && <ActivityIndicator color={colors.accent} />}
        </View>
      )}
      <Button title={buttonTitle} onPress={submit} loading={busy} disabled={!canSubmit} small style={compact ? undefined : styles.btn} />
    </View>
  );
}

function Tool({ label, onPress, bold, italic }: { label: string; onPress: () => void; bold?: boolean; italic?: boolean }) {
  const styles = useStyles(makeStyles);
  return (
    <Pressable onPress={onPress} hitSlop={6} style={({ pressed }) => [styles.tool, pressed && { opacity: 0.6 }]}>
      <Text style={[styles.toolText, bold && { fontWeight: '800' }, italic && { fontStyle: 'italic' }]}>{label}</Text>
    </Pressable>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  wrap: { padding: spacing.lg, backgroundColor: colors.card, borderBottomWidth: 1, borderBottomColor: colors.border, gap: spacing.sm },
  row: { flexDirection: 'row', alignItems: 'center', borderBottomWidth: 0, borderTopWidth: 1, borderTopColor: colors.border, padding: spacing.sm },
  input: { minHeight: 70, textAlignVertical: 'top' },
  inputRich: { minHeight: 90 },
  inputRow: { flex: 1, maxHeight: 120 },
  btn: { alignSelf: 'flex-end' },
  toolbar: { flexDirection: 'row', alignItems: 'center', flexWrap: 'wrap', gap: 2 },
  tool: { minWidth: 32, height: 32, paddingHorizontal: 6, borderRadius: radius.sm, alignItems: 'center', justifyContent: 'center', backgroundColor: colors.bg },
  toolText: { color: colors.text, fontSize: 15, fontWeight: '600' },
  linkRow: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  chips: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: spacing.sm },
  chip: { flexDirection: 'row', alignItems: 'center', gap: 6, maxWidth: 220, paddingVertical: 4, paddingLeft: 4, paddingRight: 10, borderRadius: radius.pill, borderWidth: 1, borderColor: colors.border, backgroundColor: colors.bg },
  thumb: { width: 28, height: 28, borderRadius: 14 },
  chipIcon: { fontSize: 18, paddingLeft: 4 },
  chipText: { color: colors.text, fontSize: 13, flexShrink: 1 },
  chipRemove: { color: colors.muted, fontWeight: '700', paddingLeft: 2 },
});
