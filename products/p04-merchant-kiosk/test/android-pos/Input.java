package local.poslab;

import android.app.Activity;
import android.app.Instrumentation;
import android.os.Bundle;
import android.util.Base64;
import android.view.accessibility.AccessibilityNodeInfo;
import java.nio.charset.StandardCharsets;

public final class Input extends Instrumentation {
    private static final int MAX_PAYLOAD = 4096;
    private Bundle options;

    @Override
    public void onCreate(Bundle args) {
        options = args;
        start();
    }

    private AccessibilityNodeInfo find(AccessibilityNodeInfo node, String target, String label) {
        if (node == null) {
            return null;
        }
        if (label.equals(String.valueOf(node.getContentDescription()))
                && target.equals(String.valueOf(node.getPackageName()))
                && "android.widget.EditText".contentEquals(node.getClassName())
                && node.isVisibleToUser() && node.isEnabled()) {
            return node;
        }
        for (int index = 0; index < node.getChildCount(); index++) {
            AccessibilityNodeInfo match = find(node.getChild(index), target, label);
            if (match != null) {
                return match;
            }
        }
        return null;
    }

    private boolean setTestField() {
        String packageName = options.getString("target");
        String label = options.getString("label");
        String payload = options.getString("payload");
        if (packageName == null || !packageName.matches("com\\.nu54kiosk(\\.[a-z][a-z0-9_]*)*")
                || label == null || payload == null || payload.isEmpty()
                || payload.length() > ((MAX_PAYLOAD + 2) / 3) * 4) {
            return false;
        }
        byte[] bytes = Base64.decode(payload, Base64.DEFAULT);
        if (bytes.length == 0 || bytes.length > MAX_PAYLOAD) {
            return false;
        }
        String value = new String(bytes, StandardCharsets.UTF_8);
        switch (label) {
            case "매장 발급서":
                if (!value.startsWith("{") || !value.endsWith("}")) return false;
                break;
            case "독립 확인한 서버 공개키 지문":
                if (!value.matches("[0-9a-f]{64}")) return false;
                break;
            case "POS 인계 코드":
                if (!value.matches("[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}")) return false;
                break;
            default:
                return false;
        }
        AccessibilityNodeInfo field = find(getUiAutomation().getRootInActiveWindow(), packageName, label);
        if (field == null) {
            return false;
        }
        Bundle values = new Bundle();
        values.putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, value);
        return field.performAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SET_TEXT.getId(), values);
    }

    @Override
    public void onStart() {
        Bundle result = new Bundle();
        boolean done = false;
        try {
            done = setTestField();
        } catch (RuntimeException ignored) {
            // Some platform exceptions include the original input.
        }
        result.putString("stream", done ? "\nPOS_QA_INPUT_OK\n" : "\nPOS_QA_INPUT_REJECTED\n");
        finish(done ? Activity.RESULT_OK : Activity.RESULT_CANCELED, result);
    }
}
