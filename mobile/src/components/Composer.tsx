import { Feather, MaterialCommunityIcons } from '@expo/vector-icons';
import React, { useRef, useState } from 'react';
import { Pressable, StyleSheet, TextInput, View } from 'react-native';

import { AttachmentChips } from './AttachmentChips';
import { Button, Input } from './ui';
import { type Edit, type Selection, isHttpUrl, prefixLine, wrapSelection } from '@/lib/markdownEdit';
import { useStyles, useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import type { Attachment } from '@/lib/types';
import { useAttachments } from '@/lib/useAttachments';
import { radius, spacing, type ThemeColors } from '@/theme';
import { errorMessage } from '@/lib/errors';

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
  inputRef,
  initialText = '',
  initialAttachments,
  onCancel,
  autoFocus = false,
}: {
  placeholder: string;
  buttonTitle?: string;
  onSubmit: (text: string, attachments: Attachment[]) => Promise<void>;
  multiline?: boolean;
  compact?: boolean;
  rich?: boolean;
  media?: boolean;
  inputRef?: React.RefObject<TextInput | null>; // lets a parent focus the box
  initialText?: string; // editing: start from the existing text
  initialAttachments?: Attachment[];
  onCancel?: () => void; // editing: shows a Cancel button
  autoFocus?: boolean;
}) {
  const allowMedia = rich || media;
  const styles = useStyles(makeStyles);
  const input = useRef<TextInput>(null);
  const [text, setText] = useState(initialText);
  const [selection, setSelection] = useState<Selection>({ start: initialText.length, end: initialText.length }); // caret at the end when editing
  const [linkDraft, setLinkDraft] = useState<string | null>(null); // null = link field hidden
  const [busy, setBusy] = useState(false);
  const attachments = useAttachments(initialAttachments);

  const canSubmit = (text.trim().length > 0 || attachments.attachments.length > 0) && attachments.uploading === 0 && !busy;

  const submit = async () => {
    if (!canSubmit) return;
    setBusy(true);
    try {
      await onSubmit(text.trim(), attachments.attachments);
      setText('');
      attachments.clear();
      setLinkDraft(null);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  // Applies a toolbar edit and keeps typing where it left off.
  const apply = (edit: Edit) => {
    setText(edit.text);
    setSelection(edit.selection);
    input.current?.focus();
  };

  const addLink = () => {
    const url = (linkDraft ?? '').trim();
    if (!isHttpUrl(url)) return toast.error('Links must start with http:// or https://');
    attachments.addLink(url);
    setLinkDraft(null);
  };

  const chips = allowMedia && (
    <AttachmentChips
      items={attachments.attachments}
      upload={attachments.upload}
      onRemove={attachments.removeAt}
      onCancelUpload={attachments.cancelUpload}
    />
  );

  return (
    <View style={[styles.wrap, compact && styles.wrapCompact]}>
      {rich && (
        <MarkdownToolbar
          onWrap={(before, after, fallback) => apply(wrapSelection(text, selection, before, after, fallback))}
          onPrefix={(prefix) => apply(prefixLine(text, selection, prefix))}
          onPickMedia={() => void attachments.pickMedia()}
          linkOpen={linkDraft !== null}
          onToggleLink={() => setLinkDraft((d) => (d === null ? '' : null))}
        />
      )}
      {compact && chips}
      <View style={compact ? styles.row : styles.column}>
        {compact && allowMedia && <Tool icon="image" label="Add picture or video" onPress={() => void attachments.pickMedia()} />}
        <Input
          ref={(el: TextInput | null) => {
            input.current = el;
            if (inputRef) inputRef.current = el;
          }}
          value={text}
          onChangeText={setText}
          placeholder={placeholder}
          multiline={multiline}
          autoFocus={autoFocus}
          style={[compact ? styles.inputRow : styles.input, rich && styles.inputRich]}
          onSubmitEditing={multiline ? undefined : submit}
          returnKeyType={multiline ? 'default' : 'send'}
          selection={rich ? selection : undefined}
          onSelectionChange={rich ? (e) => setSelection(e.nativeEvent.selection) : undefined}
        />
        {rich && linkDraft !== null && (
          <View style={styles.linkRow}>
            <Input
              placeholder="https://…"
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="url"
              value={linkDraft}
              onChangeText={setLinkDraft}
              onSubmitEditing={addLink}
              style={{ flex: 1 }}
            />
            <Button title="Add link" small onPress={addLink} />
          </View>
        )}
        {!compact && chips}
        {onCancel && !compact ? (
          <View style={styles.actions}>
            <Button title="Cancel" variant="secondary" small onPress={onCancel} />
            <Button title={buttonTitle} onPress={submit} loading={busy} disabled={!canSubmit} small />
          </View>
        ) : (
          <Button
            title={buttonTitle}
            onPress={submit}
            loading={busy}
            disabled={!canSubmit}
            small
            style={compact ? undefined : styles.btn}
          />
        )}
      </View>
    </View>
  );
}

// Formatting buttons: each either wraps the selection or prefixes the line.
function MarkdownToolbar({
  onWrap,
  onPrefix,
  onPickMedia,
  linkOpen,
  onToggleLink,
}: {
  onWrap: (before: string, after: string, fallback: string) => void;
  onPrefix: (prefix: string) => void;
  onPickMedia: () => void;
  linkOpen: boolean;
  onToggleLink: () => void;
}) {
  const styles = useStyles(makeStyles);
  return (
    <View style={styles.toolbar}>
      <Tool icon="bold" label="Bold" onPress={() => onWrap('**', '**', 'bold')} />
      <Tool icon="italic" label="Italic" onPress={() => onWrap('_', '_', 'italic')} />
      <Tool icon="hash" label="Heading" onPress={() => onPrefix('# ')} />
      <Tool mci="format-quote-close" label="Quote" onPress={() => onPrefix('> ')} />
      <Tool icon="code" label="Code" onPress={() => onWrap('`', '`', 'code')} />
      <Tool icon="list" label="List" onPress={() => onPrefix('- ')} />
      <Tool icon="link" label="Link" onPress={() => onWrap('[', '](https://)', 'link text')} />
      <View style={{ flex: 1 }} />
      <Tool icon="image" label="Add picture or video" onPress={onPickMedia} />
      <Tool icon="globe" label="Attach a link" active={linkOpen} onPress={onToggleLink} />
    </View>
  );
}

type FeatherName = React.ComponentProps<typeof Feather>['name'];
type MciName = React.ComponentProps<typeof MaterialCommunityIcons>['name'];

function Tool({
  icon,
  mci,
  label,
  active,
  onPress,
}: {
  icon?: FeatherName;
  mci?: MciName;
  label: string;
  active?: boolean;
  onPress: () => void;
}) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const color = active ? colors.accent : colors.text;
  return (
    <Pressable
      onPress={onPress}
      hitSlop={6}
      accessibilityLabel={label}
      style={({ pressed }) => [styles.tool, active && styles.toolActive, pressed && { opacity: 0.6 }]}
    >
      {mci ? <MaterialCommunityIcons name={mci} size={20} color={color} /> : <Feather name={icon ?? 'bold'} size={18} color={color} />}
    </Pressable>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    wrap: { padding: spacing.lg, backgroundColor: colors.card, borderBottomWidth: 1, borderBottomColor: colors.border, gap: spacing.sm },
    wrapCompact: { borderBottomWidth: 0, borderTopWidth: 1, borderTopColor: colors.border, padding: spacing.sm },
    row: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
    column: { gap: spacing.sm },
    input: { minHeight: 70, textAlignVertical: 'top' },
    inputRich: { minHeight: 90 },
    inputRow: { flex: 1, maxHeight: 120 },
    btn: { alignSelf: 'flex-end' },
    actions: { flexDirection: 'row', justifyContent: 'flex-end', gap: spacing.sm },
    toolbar: { flexDirection: 'row', alignItems: 'center', flexWrap: 'wrap', gap: 2 },
    tool: {
      minWidth: 32,
      height: 32,
      paddingHorizontal: 6,
      borderRadius: radius.sm,
      alignItems: 'center',
      justifyContent: 'center',
      backgroundColor: colors.bg,
    },
    toolActive: { backgroundColor: colors.accentSoft },
    linkRow: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  });
