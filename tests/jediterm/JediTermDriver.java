import com.jediterm.core.input.InputEvent;
import com.jediterm.core.input.KeyEvent;
import com.jediterm.core.input.KeyInputEvent;
import com.jediterm.core.input.MouseWheelEvent;
import com.jediterm.core.util.TermSize;
import com.jediterm.terminal.CursorShape;
import com.jediterm.terminal.TerminalDisplay;
import com.jediterm.terminal.TerminalOutputStream;
import com.jediterm.terminal.TtyBasedArrayDataStream;
import com.jediterm.terminal.TtyConnector;
import com.jediterm.terminal.emulator.JediEmulator;
import com.jediterm.terminal.emulator.keyboard.KeyEventProcessingResult;
import com.jediterm.terminal.emulator.keyboard.KeyEventProcessingSettings;
import com.jediterm.terminal.emulator.keyboard.TerminalKeyEventProcessor;
import com.jediterm.terminal.emulator.mouse.MouseButtonCodes;
import com.jediterm.terminal.emulator.mouse.MouseEventProcessingSettings;
import com.jediterm.terminal.emulator.mouse.MouseFormat;
import com.jediterm.terminal.emulator.mouse.MouseMode;
import com.jediterm.terminal.model.JediTerminal;
import com.jediterm.terminal.model.StyleState;
import com.jediterm.terminal.model.TerminalSelection;
import com.jediterm.terminal.model.TerminalTextBuffer;
import com.pty4j.PtyProcess;
import com.pty4j.PtyProcessBuilder;
import com.pty4j.WinSize;

import java.io.BufferedReader;
import java.io.FileDescriptor;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.io.PrintStream;
import java.io.Reader;
import java.util.Arrays;
import java.util.Base64;
import java.util.HashMap;

import static java.nio.charset.StandardCharsets.UTF_8;

/**
 * Headless JediTerm for cld's terminal contract tests: runs a command on a pty (pty4j) behind
 * JediTerm's emulator (jediterm-core), typing keys through JediTerm's own key processing.
 *
 * <p>Reads one command per line from stdin, fields separated by tabs, and answers each with one
 * JSON line on stdout: {"ok":true,"value":...}, or {"ok":false,"error":"..."} with
 * "unsupported":true when JediTerm cannot do what was asked. Commands:
 * <pre>
 * start DIR ESC_CR ARGV...  run ARGV in DIR; ESC_CR=1 makes Shift+Enter send ESC CR
 * keys KEY                  type a key named the tmux way: Enter, S-Enter, Up, Down, Escape, C-q,
 *                           or one character
 * paste BASE64              paste the text, the way JediTerm's UI does
 * wheel-up                  scroll the mouse wheel up over the screen
 * focus                     unsupported: the emulator ignores focus reporting
 * clipboard                 unsupported: the emulator does not handle OSC 52
 * title | screen | modes | running
 * output                    everything the program has written to the terminal
 * </pre>
 * End of input kills the pty and exits.
 */
public final class JediTermDriver {
  private static final int COLUMNS = 120;
  private static final int ROWS = 40;
  /** The key char of a key that types no character, as java.awt.event.KeyEvent has it. */
  private static final char CHAR_UNDEFINED = '\uffff';

  private final Display display = new Display();
  private final StyleState style = new StyleState();
  private final TerminalTextBuffer buffer = new TerminalTextBuffer(COLUMNS, ROWS, style);
  private final JediTerminal terminal = new JediTerminal(display, buffer, style);
  private final StringBuilder output = new StringBuilder();
  private KeyEventProcessingSettings keySettings;
  private PtyProcess process;
  private Connector connector;

  public static void main(String[] args) throws IOException {
    new JediTermDriver().serve();
  }

  private void serve() throws IOException {
    BufferedReader commands = new BufferedReader(new InputStreamReader(System.in, UTF_8));
    PrintStream replies = new PrintStream(new FileOutputStream(FileDescriptor.out), true, UTF_8);
    for (String line; (line = commands.readLine()) != null; ) {
      String reply;
      try {
        reply = "{\"ok\":true,\"value\":" + handle(line.split("\t", -1)) + "}";
      }
      catch (UnsupportedOperationException e) {
        reply = "{\"ok\":false,\"unsupported\":true,\"error\":" + quote(e.getMessage()) + "}";
      }
      catch (Exception e) {
        e.printStackTrace();
        reply = "{\"ok\":false,\"error\":" + quote(e.toString()) + "}";
      }
      replies.println(reply);
    }
    if (process != null) {
      process.destroyForcibly();
    }
  }

  private String handle(String[] command) throws IOException {
    switch (command[0]) {
      case "start":
        start(command[1], command[2].equals("1"), Arrays.copyOfRange(command, 3, command.length));
        return "null";
      case "keys":
        connector.write(encode(command[1]));
        return "null";
      case "paste":
        paste(new String(Base64.getDecoder().decode(command[1]), UTF_8));
        return "null";
      case "wheel-up":
        MouseEventProcessingSettings settings =
          new MouseEventProcessingSettings(true, buffer.isUsingAlternateBuffer(), false);
        // JediTerm's UI turns an upward wheel rotation into SCROLLDOWN, which it reports as
        // xterm's button 64 (wheel up).
        MouseWheelEvent up = new MouseWheelEvent(MouseButtonCodes.SCROLLDOWN, 0, -1);
        if (!terminal.onMouseEvent(10, 10, up, settings)) {
          throw new IllegalStateException("the program has not enabled mouse reporting");
        }
        return "null";
      case "focus":
        throw new UnsupportedOperationException("the emulator ignores focus reporting (DECSET 1004)");
      case "clipboard":
        throw new UnsupportedOperationException("the emulator does not handle OSC 52");
      case "title":
        return quote(display.title);
      case "screen":
        return quote(buffer.getScreenLines());
      case "modes":
        return "{\"alt_screen\":" + buffer.isUsingAlternateBuffer() +
               ",\"mouse\":" + (display.mouseMode != MouseMode.MOUSE_REPORTING_NONE) +
               ",\"cursor\":" + display.cursorVisible + "}";
      case "running":
        return String.valueOf(process != null && process.isAlive());
      case "output":
        synchronized (output) {
          return quote(output.toString());
        }
      default:
        throw new IllegalArgumentException("unknown command " + command[0]);
    }
  }

  private void start(String directory, boolean shiftEnterSendsEscCr, String[] argv) throws IOException {
    keySettings = new KeyEventProcessingSettings(shiftEnterSendsEscCr, false, true);
    process = new PtyProcessBuilder(argv)
      .setEnvironment(new HashMap<>(System.getenv()))
      .setDirectory(directory)
      .setInitialColumns(COLUMNS)
      .setInitialRows(ROWS)
      .start();
    connector = new Connector(process, output);
    terminal.setTerminalOutput(new TerminalOutputStream() {
      @Override
      public void sendBytes(byte[] response, boolean userInput) {
        connector.writeQuietly(response);
      }

      @Override
      public void sendString(String string, boolean userInput) {
        connector.writeQuietly(string.getBytes(UTF_8));
      }
    });
    JediEmulator emulator = new JediEmulator(new TtyBasedArrayDataStream(connector), terminal);
    Thread thread = new Thread(() -> {
      try {
        while (emulator.hasNext()) {
          emulator.next();
        }
      }
      catch (Exception e) {
        if (process.isAlive()) {
          e.printStackTrace();
        }
      }
    }, "emulator");
    thread.setDaemon(true);
    thread.start();
  }

  /**
   * Pastes the way JediTerm's UI does (TerminalPanel.pasteFromClipboard outside Windows): line
   * breaks become carriage returns, and the text is bracketed if the program asked for it.
   */
  private void paste(String text) throws IOException {
    text = text.replace("\r\n", "\n").replace('\n', '\r');
    if (display.bracketedPaste) {
      text = "\u001b[200~" + text + "\u001b[201~";
    }
    connector.write(text);
  }

  /**
   * Types a key the way JediTerm's UI does: a key-pressed event, or key-typed for a character.
   * The arrows have codes of their own, which JediTerm's encoder turns into ANSI or application
   * cursor sequences as the program asked. Its encoder has no code for Escape: JediTerm passes a
   * pressed key without one on when its key char is a control character, as for a Ctrl+letter.
   */
  private byte[] encode(String key) {
    KeyInputEvent event;
    if (key.equals("Enter") || key.equals("S-Enter")) {
      int modifiers = key.equals("S-Enter") ? InputEvent.SHIFT_DOWN_MASK : 0;
      event = new KeyInputEvent(KeyInputEvent.Type.PRESSED, KeyEvent.VK_ENTER, '\n', modifiers);
    }
    else if (key.equals("Up") || key.equals("Down")) {
      int code = key.equals("Up") ? KeyEvent.VK_UP : KeyEvent.VK_DOWN;
      event = new KeyInputEvent(KeyInputEvent.Type.PRESSED, code, CHAR_UNDEFINED, 0);
    }
    else if (key.equals("Escape")) {
      event = new KeyInputEvent(KeyInputEvent.Type.PRESSED, KeyEvent.VK_ESCAPE, '\u001b', 0);
    }
    else if (key.length() == 3 && key.startsWith("C-")) {
      char letter = Character.toUpperCase(key.charAt(2));
      event = new KeyInputEvent(KeyInputEvent.Type.PRESSED, letter, (char)(letter & 0x1f), InputEvent.CTRL_DOWN_MASK);
    }
    else if (key.length() == 1) {
      event = new KeyInputEvent(KeyInputEvent.Type.TYPED, 0, key.charAt(0), 0);
    }
    else {
      throw new IllegalArgumentException("unknown key " + key);
    }
    KeyEventProcessingResult result = TerminalKeyEventProcessor.processKey(event, terminal, keySettings);
    if (result instanceof KeyEventProcessingResult.BytesResult) {
      return ((KeyEventProcessingResult.BytesResult)result).getBytes();
    }
    if (result instanceof KeyEventProcessingResult.StringResult) {
      return ((KeyEventProcessingResult.StringResult)result).getString().getBytes(UTF_8);
    }
    throw new IllegalStateException("JediTerm does not handle the key " + key);
  }

  private static String quote(String text) {
    StringBuilder json = new StringBuilder("\"");
    for (char c : text.toCharArray()) {
      if (c == '"' || c == '\\') {
        json.append('\\').append(c);
      }
      else if (c < 0x20) {
        json.append(String.format("\\u%04x", (int)c));
      }
      else {
        json.append(c);
      }
    }
    return json.append('"').toString();
  }

  /** Records what the emulator tells the screen; nothing is drawn. */
  private static final class Display implements TerminalDisplay {
    volatile String title = "";
    volatile MouseMode mouseMode = MouseMode.MOUSE_REPORTING_NONE;
    volatile boolean bracketedPaste;
    volatile boolean cursorVisible = true;

    @Override public void setCursor(int x, int y) { }
    @Override public void setCursorShape(CursorShape cursorShape) { }
    @Override public void beep() { }
    @Override public void scrollArea(int scrollRegionTop, int scrollRegionSize, int dy) { }
    @Override public void setCursorVisible(boolean isCursorVisible) { cursorVisible = isCursorVisible; }
    @Override public void useAlternateScreenBuffer(boolean useAlternateScreenBuffer) { }
    @Override public String getWindowTitle() { return title; }
    @Override public void setWindowTitle(String windowTitle) { title = windowTitle; }
    @Override public TerminalSelection getSelection() { return null; }
    @Override public void terminalMouseModeSet(MouseMode mode) { mouseMode = mode; }
    @Override public void setBracketedPasteMode(boolean enabled) { bracketedPaste = enabled; }
    @Override public void setMouseFormat(MouseFormat mouseFormat) { }
    @Override public boolean ambiguousCharsAreDoubleWidth() { return false; }
  }

  /** The pty, recording everything the program writes to it into a log. */
  private static final class Connector implements TtyConnector {
    private final PtyProcess process;
    private final Reader reader;
    private final OutputStream output;
    private final StringBuilder log;

    Connector(PtyProcess process, StringBuilder log) {
      this.process = process;
      this.reader = new InputStreamReader(process.getInputStream(), UTF_8);
      this.output = process.getOutputStream();
      this.log = log;
    }

    @Override
    public int read(char[] buf, int offset, int length) throws IOException {
      int count = reader.read(buf, offset, length);
      if (count > 0) {
        synchronized (log) {
          log.append(buf, offset, count);
        }
      }
      return count;
    }

    @Override
    public synchronized void write(byte[] bytes) throws IOException {
      output.write(bytes);
      output.flush();
    }

    @Override
    public void write(String string) throws IOException {
      write(string.getBytes(UTF_8));
    }

    void writeQuietly(byte[] bytes) {
      try {
        write(bytes);
      }
      catch (IOException ignored) {
        // the program exited; nobody reads the response
      }
    }

    @Override
    public boolean isConnected() {
      return process.isAlive();
    }

    @Override
    public void resize(TermSize termSize) {
      process.setWinSize(new WinSize(termSize.getColumns(), termSize.getRows()));
    }

    @Override
    public int waitFor() throws InterruptedException {
      return process.waitFor();
    }

    @Override
    public boolean ready() throws IOException {
      return reader.ready();
    }

    @Override
    public String getName() {
      return "cld";
    }

    @Override
    public void close() {
      process.destroy();
    }
  }
}
