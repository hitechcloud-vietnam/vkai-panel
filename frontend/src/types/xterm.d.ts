declare module 'xterm' {
  export interface ITerminalOptions {
    theme?: ITheme;
    fontFamily?: string;
    fontSize?: number;
    lineHeight?: number;
    cursorBlink?: boolean;
    cursorStyle?: 'block' | 'underline' | 'bar';
    cursorWidth?: number;
    scrollback?: number;
    tabStopWidth?: number;
    bellStyle?: 'none' | 'sound';
    allowTransparency?: boolean;
    convertEol?: boolean;
  }

  export interface ITheme {
    background?: string;
    foreground?: string;
    cursor?: string;
    cursorAccent?: string;
    selectionBackground?: string;
    selectionForeground?: string;
    black?: string;
    red?: string;
    green?: string;
    yellow?: string;
    blue?: string;
    magenta?: string;
    cyan?: string;
    white?: string;
    brightBlack?: string;
    brightRed?: string;
    brightGreen?: string;
    brightYellow?: string;
    brightBlue?: string;
    brightMagenta?: string;
    brightCyan?: string;
    brightWhite?: string;
  }

  export interface IDisposable {
    dispose(): void;
  }

  export class Terminal {
    constructor(options?: ITerminalOptions);
    cols: number;
    rows: number;
    open(container: HTMLElement): void;
    write(data: string): void;
    writeln(data: string): void;
    clear(): void;
    focus(): void;
    dispose(): void;
    loadAddon(addon: any): void;
    onData(callback: (data: string) => void): IDisposable;
    onResize(callback: (size: { cols: number; rows: number }) => void): IDisposable;
  }
}

declare module 'xterm-addon-fit' {
  export class FitAddon {
    fit(): void;
    dispose(): void;
  }
}

declare module 'xterm-addon-web-links' {
  export class WebLinksAddon {
    constructor();
    dispose(): void;
  }
}

declare module 'xterm/css/xterm.css' {
  const content: string;
  export default content;
}
