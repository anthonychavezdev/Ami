import { Alert } from 'react-native';

// Yes/no confirmation before a destructive action.
export function confirmAction(title: string, message: string | undefined, action: string, onConfirm: () => void) {
  Alert.alert(title, message, [
    { text: 'Cancel', style: 'cancel' },
    { text: action, style: 'destructive', onPress: onConfirm },
  ]);
}
