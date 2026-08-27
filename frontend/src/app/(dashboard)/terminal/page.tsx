'use client';

import { useState } from 'react';
import { Terminal as TerminalIcon } from 'lucide-react';

export default function TerminalPage() {
  const [command, setCommand] = useState('');
  const [output, setOutput] = useState<string[]>([
    'Welcome to vKAI Terminal',
    'Type commands to execute on the server',
    '',
  ]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && command.trim()) {
      setOutput([...output, `$ ${command}`, 'Command execution coming soon...', '']);
      setCommand('');
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-dark-50">Terminal</h1>
        <p className="text-dark-400 mt-1">Execute commands on your servers</p>
      </div>

      <div className="card p-0 overflow-hidden">
        <div className="bg-dark-950 p-4 font-mono text-sm h-[500px] flex flex-col">
          <div className="flex-1 overflow-y-auto space-y-1">
            {output.map((line, i) => (
              <div key={i} className="text-dark-300">
                {line}
              </div>
            ))}
          </div>
          <div className="flex items-center gap-2 mt-4 pt-4 border-t border-dark-700">
            <span className="text-primary-400">$</span>
            <input
              type="text"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              onKeyDown={handleKeyDown}
              className="flex-1 bg-transparent text-dark-100 outline-none font-mono"
              placeholder="Type a command..."
            />
          </div>
        </div>
      </div>
    </div>
  );
}
