package org.nu54.qa.subscriptionclip;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.os.Bundle;
import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;

public final class Clip extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        File input = new File(getFilesDir(), "fixture.txt");
        ClipboardManager clipboard = getSystemService(ClipboardManager.class);
        try (FileInputStream stream = new FileInputStream(input)) {
            int size = (int) input.length();
            if (size < 1 || size > 16384) throw new IOException("Invalid synthetic enrollment size");
            byte[] data = new byte[size];
            new java.io.DataInputStream(stream).readFully(data);
            clipboard.setPrimaryClip(ClipData.newPlainText("Disposable lab enrollment",
                new String(data, java.nio.charset.StandardCharsets.UTF_8)));
        } catch (IOException error) {
            clipboard.clearPrimaryClip();
        } finally {
            input.delete();
            finish();
        }
    }
}
