import React from 'react';
import { motion } from 'framer-motion';
import { Zap, Cpu } from 'lucide-react';
import type { ClusterStatus } from '../../api';

interface TopologyProps {
  status: ClusterStatus;
}

export const Topology: React.FC<TopologyProps> = ({ status }) => {
  const nodes = Object.values(status.nodes);
  const centerX = 200;
  const centerY = 150;
  const radius = 110;

  return (
    <div className="w-full h-full min-h-[320px] flex items-center justify-center bg-muted/30 rounded-xl border border-dashed relative overflow-hidden">
      <div className="absolute inset-0 bg-[radial-gradient(var(--border)_1px,transparent_1px)] [background-size:24px_24px] opacity-50" />
      <svg viewBox="0 0 400 300" className="w-full max-w-[500px] h-auto overflow-visible relative z-10">
        <circle cx={centerX} cy={centerY} r={22} className="fill-primary stroke-background stroke-2 shadow-lg" />
        <g transform={`translate(${centerX - 10}, ${centerY - 10})`} className="text-primary-foreground pointer-events-none">
          <Zap size={20} fill="currentColor" fillOpacity={0.2} />
        </g>

        {nodes.map((node, i) => {
          const angle = (i / nodes.length) * 2 * Math.PI - Math.PI / 2;
          const x = centerX + Math.cos(angle) * radius;
          const y = centerY + Math.sin(angle) * radius;
          const isActive = (node.active_models?.length || 0) > 0;
          const workload = status.node_workloads?.[node.address] || 0;

          return (
            <g key={node.address}>
              <line 
                x1={centerX} y1={centerY} x2={x} y2={y} 
                className={`transition-all duration-500 ${node.state === 2 ? 'stroke-destructive/30' : isActive ? 'stroke-primary' : 'stroke-border'}`} 
                strokeWidth={1 + workload * 1.5}
                strokeDasharray={node.draining || node.state === 2 ? "4 2" : "0"}
              />
              {workload > 0 && (
                <motion.circle
                  r={2 + workload} className="fill-primary"
                  animate={{ cx: [centerX, x], cy: [centerY, y] }}
                  transition={{ duration: Math.max(0.5, 2 - workload * 0.3), repeat: Infinity, ease: "linear", delay: i * 0.2 }}
                />
              )}
              <circle cx={x} cy={y} r={16} className={`stroke-2 transition-colors duration-500 ${node.has_gpu ? 'fill-indigo-50 stroke-indigo-500' : 'fill-slate-50 stroke-slate-400'}`} />
              <g transform={`translate(${x - 8}, ${y - 8})`} className={node.has_gpu ? "text-indigo-600" : "text-slate-600"}>
                {node.has_gpu ? <Zap size={16} /> : <Cpu size={16} />}
              </g>
              <text x={x} y={y + 28} textAnchor="middle" className="text-[7px] font-black fill-muted-foreground uppercase tracking-tighter">
                {node.id.split('-').pop()}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
};
