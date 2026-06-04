/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 主色调 - 星光蓝（白洞/黑洞主题共用）
        primary: {
          50: '#eef4fe',
          100: '#d9e6fd',
          200: '#bcd2fb',
          300: '#93b8f5',
          400: '#5e93f0',
          500: '#3b76e8',
          600: '#2b5cce',
          700: '#2749a6',
          800: '#243f85',
          900: '#22386b',
          950: '#172445'
        },
        // 辅助色 - 星云靛紫 (少量点缀)
        accent: {
          50: '#f5f3ff',
          100: '#ede9fe',
          200: '#ddd6fe',
          300: '#c4b5fd',
          400: '#a78bfa',
          500: '#8b5cf6',
          600: '#7c3aed',
          700: '#6d28d9',
          800: '#5b21b6',
          900: '#4c1d95',
          950: '#2e1065'
        },
        // 黑洞夜间背景梯度 - navy
        dark: {
          50: '#e7ecf5',
          100: '#c6d0e3',
          200: '#9fadc8',
          300: '#6f7fa3',
          400: '#48577d',
          500: '#2f3c5e',
          600: '#1e2742',
          700: '#1e2742',
          800: '#161d33',
          900: '#0e1426',
          950: '#080d1c'
        },
        // 覆盖内置 gray 为冷调中性 (全站 gray-* 硬编码自动适配深空)
        gray: {
          50: '#f1f5f9',
          100: '#e6ecf7',
          200: '#cdd6e3',
          300: '#aab6cc',
          400: '#7e8cab',
          500: '#64748b',
          600: '#475569',
          700: '#28324d',
          800: '#1a2238',
          900: '#111a30',
          950: '#0a1124'
        },
        // slate 同步冷调
        slate: {
          50: '#f1f5f9',
          100: '#e6ecf7',
          200: '#cdd6e3',
          300: '#aab6cc',
          400: '#7e8cab',
          500: '#64748b',
          600: '#475569',
          700: '#28324d',
          800: '#1a2238',
          900: '#111a30',
          950: '#0a1124'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 12px 40px rgba(31, 81, 137, 0.14)',
        'glass-sm': '0 6px 20px rgba(31, 81, 137, 0.1)',
        glow: '0 0 20px rgba(59, 118, 232, 0.3)',
        'glow-lg': '0 0 40px rgba(59, 118, 232, 0.4)',
        card: '0 1px 3px rgba(31, 81, 137, 0.08), 0 8px 24px rgba(31, 81, 137, 0.08)',
        'card-hover': '0 12px 40px rgba(31, 81, 137, 0.14)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #3b76e8 0%, #2b5cce 100%)',
        'gradient-dark': 'linear-gradient(135deg, #161d33 0%, #080d1c 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.06) 0%, rgba(255,255,255,0.02) 100%)',
        // 兼容旧背景类；页面主背景由 style.css 的 cosmic-shell 控制。
        'mesh-gradient':
          'radial-gradient(at 82% -10%, rgba(59, 118, 232, 0.10) 0px, transparent 55%), radial-gradient(at 8% 115%, rgba(139, 92, 246, 0.08) 0px, transparent 55%), radial-gradient(at 50% 50%, rgba(43, 92, 206, 0.05) 0px, transparent 60%)',
        nebula:
          'radial-gradient(at 80% -8%, rgba(59, 118, 232, 0.20) 0px, transparent 55%), radial-gradient(at 10% 112%, rgba(139, 92, 246, 0.16) 0px, transparent 55%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate',
        twinkle: 'twinkle 4s ease-in-out infinite alternate',
        twinkle2: 'twinkle2 6s ease-in-out infinite alternate',
        drift: 'drift 120s linear infinite',
        drift2: 'drift2 170s linear infinite'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(59, 118, 232, 0.25)' },
          '100%': { boxShadow: '0 0 30px rgba(59, 118, 232, 0.45)' }
        },
        twinkle: {
          '0%': { opacity: '0.45' },
          '100%': { opacity: '1' }
        },
        twinkle2: {
          '0%': { opacity: '0.3' },
          '100%': { opacity: '0.85' }
        },
        drift: {
          '0%': { transform: 'translate(0, 0)' },
          '100%': { transform: 'translate(3%, 2%)' }
        },
        drift2: {
          '0%': { transform: 'translate(0, 0)' },
          '100%': { transform: 'translate(-2.5%, -1.5%)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
