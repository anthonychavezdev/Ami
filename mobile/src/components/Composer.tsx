import { Feather, MaterialCommunityIcons } from '@expo/vector-icons';
import * as ImagePicker from 'expo-image-picker';
import React, { useRef, useState } from 'react';
import { ActivityIndicator, Image, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';

import { Button, Input } from './ui';
import { api, mediaUrl } from '@/lib/api';
import { useStyles, useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import type { Attachment } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

type Selection = { start: number; end: number };

// Generic "write something and submit" box used for posts, comments and chat.
// `rich` adds a Markdown toolbar plus picture, video and link attachments;
// `media` adds just the picture/video picker (used by chats).
export function Composer({
  placeholder,
  buttonTitle = 'Post',
  onSubmit,
  multiline = true,
  compact = false,
  rich = false,
  media = false,
}: {
  placeholder: string;
  buttonTitle?: string;
  onSubmit: (text: string, attachments: Attachment[]) => Promise<void>;
  multiline?: boolean;
  compact?: boolean;
  rich?: boolean;
  media?: boolean;
}) {
  const allowMedia = rich || media;
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
      toast.error(e instanceof Error ? e.message : String(e));
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
        toast.error(`Upload failed: ${e instanceof Error ? e.message : String(e)}`);
      } finally {
        setUploading((n) => n - 1);
      }
    }
  };

  const addLink = () => {
    const url = (linkDraft ?? '').trim();
    if (!/^https?:\/\/\S+$/i.test(url)) {
      toast.error('Links must start with http:// or https://');
      return;
    }
    setAttachments((prev) => [...prev, { type: 'link', url }]);
    setLinkDraft(null);
  };

  const chips = allowMedia && (attachments.length > 0 || uploading > 0) && (
    <View style={styles.chips}>
      {attachments.map((a, i) => (
        <View key={`${a.url}-${i}`} style={styles.chip}>
          {a.type === 'image' ? <Image source={{ uri: mediaUrl(a) }} style={styles.thumb} /> : <Feather name={a.type === 'video' ? 'video' : 'link'} size={16} color={colors.muted} style={styles.chipIcon} />}
          <Text numberOfLines={1} style={styles.chipText}>
            {a.type === 'link' ? a.url.replace(/^https?:\/\//, '') : a.type}
          </Text>
          <Pressable onPress={() => setAttachments((prev) => prev.filter((_, j) => j !== i))} hitSlop={8} accessibilityLabel="Remove attachment">
            <Feather name="x" size={14} color={colors.muted} />
          </Pressable>
        </View>
      ))}
      {uploading > 0 && <ActivityIndicator color={colors.accent} />}
    </View>
  );

  return (
    <View style={[styles.wrap, compact && styles.wrapCompact]}>
      {rich && (
        <View style={styles.toolbar}>
          <Tool icon="bold" label="Bold" onPress={() => wrap('**', '**', 'bold')} />
          <Tool icon="italic" label="Italic" onPress={() => wrap('_', '_', 'italic')} />
          <Tool icon="hash" label="Heading" onPress={() => prefixLine('# ')} />
          <Tool mci="format-quote-close" label="Quote" onPress={() => prefixLine('> ')} />
          <Tool icon="code" label="Code" onPress={() => wrap('`', '`', 'code')} />
          <Tool icon="list" label="List" onPress={() => prefixLine('- ')} />
          <Tool icon="link" label="Link" onPress={() => wrap('[', '](https://)', 'link text')} />
          <View style={{ flex: 1 }} />
          <Tool icon="image" label="Add picture or video" onPress={() => void pickMedia()} />
          <Tool icon="globe" label="Attach a link" active={linkDraft !== null} onPress={() => setLinkDraft((d) => (d === null ? '' : null))} />
        </View>
      )}
      {compact && chips}
      <View style={compact ? styles.row : styles.column}>
        {compact && allowMedia && <Tool icon="image" label="Add picture or video" onPress={() => void pickMedia()} />}
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
        {!compact && chips}
        <Button title={buttonTitle} onPress={submit} loading={busy} disabled={!canSubmit} small style={compact ? undefined : styles.btn} />
      </View>
    </View>
  );
}

type FeatherName = React.ComponentProps<typeof Feather>['name'];
type MciName = React.ComponentProps<typeof MaterialCommunityIcons>['name'];

function Tool({ icon, mci, label, active, onPress }: { icon?: FeatherName; mci?: MciName; label: string; active?: boolean; onPress: () => void }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const color = active ? colors.accent : colors.text;
  return (
    <Pressable onPress={onPress} hitSlop={6} accessibilityLabel={label} style={({ pressed }) => [styles.tool, active && styles.toolActive, pressed && { opacity: 0.6 }]}>
      {mci ? <MaterialCommunityIcons name={mci} size={20} color={color} /> : <Feather name={icon ?? 'bold'} size={18} color={color} />}
    </Pressable>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  wrap: { padding: spacing.lg, backgroundColor: colors.card, borderBottomWidth: 1, borderBottomColor: colors.border, gap: spacing.sm },
  wrapCompact: { borderBottomWidth: 0, borderTopWidth: 1, borderTopColor: colors.border, padding: spacing.sm },
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  column: { gap: spacing.sm },
  input: { minHeight: 70, textAlignVertical: 'top' },
  inputRich: { minHeight: 90 },
  inputRow: { flex: 1, maxHeight: 120 },
  btn: { alignSelf: 'flex-end' },
  toolbar: { flexDirection: 'row', alignItems: 'center', flexWrap: 'wrap', gap: 2 },
  tool: { minWidth: 32, height: 32, paddingHorizontal: 6, borderRadius: radius.sm, alignItems: 'center', justifyContent: 'center', backgroundColor: colors.bg },
  toolActive: { backgroundColor: colors.accentSoft },
  linkRow: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  chips: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: spacing.sm },
  chip: { flexDirection: 'row', alignItems: 'center', gap: 6, maxWidth: 220, paddingVertical: 4, paddingLeft: 4, paddingRight: 10, borderRadius: radius.pill, borderWidth: 1, borderColor: colors.border, backgroundColor: colors.bg },
  thumb: { width: 28, height: 28, borderRadius: 14 },
  chipIcon: { paddingLeft: 6 },
  chipText: { color: colors.text, fontSize: 13, flexShrink: 1 },
});
