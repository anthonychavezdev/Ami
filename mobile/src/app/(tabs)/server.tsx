import { useFocusEffect } from 'expo-router';
import React, { useCallback, useState } from 'react';
import { RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';

import { Card, Empty, Loading, SectionLabel, WideColumn } from '@/components/ui';
import { api } from '@/lib/api';
import { formatBytes } from '@/lib/bytes';
import { errorMessage } from '@/lib/errors';
import { useStyles, useTheme } from '@/lib/theme';
import type { ServerStats } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

const REFRESH_EVERY = 10_000;

// What the server has and what it is spending: the disk its uploads sit on
// and your share of them, the processor's load, the memory. Reloads every ten
// seconds while on screen, since load and memory move.
export default function ServerScreen() {
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const [stats, setStats] = useState<ServerStats | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async () => {
    try {
      setStats(await api.stats());
      setError(null);
    } catch (e) {
      setError(errorMessage(e));
    }
  }, []);
  useFocusEffect(
    useCallback(() => {
      void load();
      const timer = setInterval(() => void load(), REFRESH_EVERY);
      return () => clearInterval(timer);
    }, [load]),
  );
  const refresh = async () => {
    setRefreshing(true);
    await load();
    setRefreshing(false);
  };

  if (!stats) return error ? <Empty text={error} /> : <Loading />;
  const { storage, cpu, memory } = stats;
  const diskUsed = storage.disk_total - storage.disk_free;
  const memoryUsed = memory.total - memory.available;
  const unknown = 'Not known on this system';
  return (
    <WideColumn>
      <ScrollView
        contentContainerStyle={styles.content}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => void refresh()} tintColor={colors.accent} />}
      >
        <Text style={styles.intro}>The computer this Ami runs on, and your part of it.</Text>

        <SectionLabel>Storage</SectionLabel>
        <Card style={styles.card}>
          <Meter
            label="Your pictures and videos"
            value={storage.yours}
            max={storage.your_limit || storage.uploads}
            warnAt={storage.your_limit ? 0.9 : undefined}
            detail={
              storage.your_limit
                ? `${formatBytes(storage.yours)} of the ${formatBytes(storage.your_limit)} you may upload`
                : `${formatBytes(storage.yours)} of the ${formatBytes(storage.uploads)} everyone has uploaded`
            }
          />
          <Text style={styles.hint}>Everything you have uploaded, on your posts, your messages and your profile.</Text>
          <Meter
            label="Disk"
            value={diskUsed}
            max={storage.disk_total}
            warnAt={0.9}
            detail={
              storage.disk_total
                ? `${formatBytes(diskUsed)} used of ${formatBytes(storage.disk_total)} · ${formatBytes(storage.disk_free)} free`
                : unknown
            }
          />
          <Row label="Everyone's pictures and videos" value={formatBytes(storage.uploads)} />
          <Row label="Database" value={formatBytes(storage.database)} />
          <Row label="Largest upload allowed" value={formatBytes(storage.upload_limit)} />
        </Card>

        <SectionLabel>Processor</SectionLabel>
        <Card style={styles.card}>
          <Meter
            label="Load"
            value={cpu.load_1}
            max={cpu.cores}
            warnAt={0.9}
            detail={
              cpu.cores ? `${cpu.load_1.toFixed(2)} over the last minute · ${cpu.cores} ${cpu.cores === 1 ? 'core' : 'cores'}` : unknown
            }
          />
          <Row label="Last 5 and 15 minutes" value={`${cpu.load_5.toFixed(2)} · ${cpu.load_15.toFixed(2)}`} />
          <Row label="This server" value={`${cpu.server_percent.toFixed(1)}% of one core`} />
        </Card>

        <SectionLabel>Memory</SectionLabel>
        <Card style={styles.card}>
          <Meter
            label="In use"
            value={memoryUsed}
            max={memory.total}
            warnAt={0.9}
            detail={
              memory.total
                ? `${formatBytes(memoryUsed)} of ${formatBytes(memory.total)} · ${formatBytes(memory.available)} available`
                : unknown
            }
          />
          <Row label="This server" value={formatBytes(memory.server)} />
        </Card>
        <Text style={styles.hint}>Updated every ten seconds while this tab is open.</Text>
      </ScrollView>
    </WideColumn>
  );
}

// A labelled bar: value out of max, a line of detail under it. Red past
// warnAt (a share of max); empty, with no percentage, when max is unknown.
function Meter({ label, value, max, detail, warnAt = 2 }: { label: string; value: number; max: number; detail: string; warnAt?: number }) {
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const share = max > 0 ? Math.min(1, Math.max(0, value / max)) : 0;
  const percent = Math.round(share * 100);
  return (
    <View
      style={styles.meter}
      accessibilityRole="progressbar"
      accessibilityLabel={label}
      accessibilityValue={{ min: 0, max: 100, now: percent }}
    >
      <View style={styles.row}>
        <Text style={styles.label}>{label}</Text>
        {max > 0 && <Text style={styles.percent}>{percent}%</Text>}
      </View>
      <View style={styles.track}>
        <View style={[styles.fill, { width: `${percent}%`, backgroundColor: share >= warnAt ? colors.danger : colors.accent }]} />
      </View>
      <Text style={styles.detail}>{detail}</Text>
    </View>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  const styles = useStyles(makeStyles);
  return (
    <View style={styles.row}>
      <Text style={styles.label}>{label}</Text>
      <Text style={styles.value}>{value}</Text>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    content: { padding: spacing.lg, gap: spacing.sm, paddingBottom: spacing.xl },
    intro: { color: colors.muted, marginBottom: spacing.sm },
    card: { gap: spacing.md, marginBottom: spacing.lg },
    hint: { color: colors.muted, fontSize: 13 },
    meter: { gap: 6 },
    row: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: spacing.md },
    label: { color: colors.text, fontWeight: '600', flexShrink: 1 },
    value: { color: colors.muted, fontVariant: ['tabular-nums'] },
    percent: { color: colors.muted, fontVariant: ['tabular-nums'] },
    track: { height: 8, borderRadius: radius.pill, backgroundColor: colors.border, overflow: 'hidden' },
    fill: { height: '100%', borderRadius: radius.pill },
    detail: { color: colors.muted, fontSize: 13 },
  });
