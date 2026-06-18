/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Claude-inspired warm neutral system. Overriding gray keeps existing utility classes
        // visually consistent without forcing a broad component-by-component rewrite.
        gray: {
          50: '#faf8f3',
          100: '#f3eee7',
          200: '#e7ded2',
          300: '#d6c8b8',
          400: '#aa9a8c',
          500: '#7d6f63',
          600: '#5f554c',
          700: '#49413b',
          800: '#332e29',
          900: '#24201d',
          950: '#171411'
        },
        primary: {
          50: '#fff7ed',
          100: '#ffedd5',
          200: '#fed7aa',
          300: '#f8b77d',
          400: '#e8915d',
          500: '#d66f45',
          600: '#bd5733',
          700: '#99422a',
          800: '#7c3828',
          900: '#653125',
          950: '#351712'
        },
        accent: {
          50: '#fbfaf8',
          100: '#f4efe8',
          200: '#e8dccd',
          300: '#d7c3ac',
          400: '#ba9979',
          500: '#9e7452',
          600: '#815b3e',
          700: '#674632',
          800: '#4f362a',
          900: '#382620',
          950: '#211613'
        },
        dark: {
          50: '#faf8f3',
          100: '#f3eee7',
          200: '#e7ded2',
          300: '#d6c8b8',
          400: '#a69686',
          500: '#7c6f63',
          600: '#5f554c',
          700: '#3f3832',
          800: '#2d2925',
          900: '#211e1a',
          950: '#151310'
        }
      },
      fontFamily: {
        sans: [
          'Inter',
          'ui-sans-serif',
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
        glass: '0 24px 80px rgba(54, 39, 26, 0.10)',
        'glass-sm': '0 12px 32px rgba(54, 39, 26, 0.08)',
        glow: '0 0 28px rgba(214, 111, 69, 0.24)',
        'glow-lg': '0 0 56px rgba(214, 111, 69, 0.32)',
        soft: '0 1px 2px rgba(54, 39, 26, 0.06), 0 8px 24px rgba(54, 39, 26, 0.08)',
        card: '0 1px 2px rgba(54, 39, 26, 0.05), 0 12px 36px rgba(54, 39, 26, 0.08)',
        'card-hover': '0 18px 56px rgba(54, 39, 26, 0.13)',
        floating: '0 24px 80px rgba(0, 0, 0, 0.16)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.16)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #d66f45 0%, #99422a 100%)',
        'gradient-dark': 'linear-gradient(135deg, #2d2925 0%, #151310 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,250,243,0.82) 0%, rgba(250,248,243,0.56) 100%)',
        'mesh-gradient':
          'radial-gradient(at 12% 12%, rgba(214,111,69,0.15) 0px, transparent 34%), radial-gradient(at 84% 8%, rgba(158,116,82,0.12) 0px, transparent 32%), radial-gradient(at 52% 88%, rgba(153,66,42,0.10) 0px, transparent 38%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
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
          '0%': { boxShadow: '0 0 20px rgba(214, 111, 69, 0.22)' },
          '100%': { boxShadow: '0 0 32px rgba(214, 111, 69, 0.34)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem',
        '5xl': '2.5rem'
      }
    }
  },
  plugins: []
}
