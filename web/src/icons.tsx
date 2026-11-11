import type { SVGProps } from "react";
import type { Role } from "./types";

type IconProps = SVGProps<SVGSVGElement>;

export function MerlinEmblem(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <circle cx="12" cy="8.5" r="4.2" />
      <path d="M12 6.3L12.8 7.8L14.3 8.5L12.8 9.2L12 10.7L11.2 9.2L9.7 8.5L11.2 7.8L12 6.3Z" />
      <path d="M12 12.8V21.5" />
      <path d="M7.2 5.2C5.2 6.9 4.5 9.8 5.8 12.2" />
      <path d="M16.8 5.2C18.8 6.9 19.5 9.8 18.2 12.2" />
      <path d="M9.5 16.5H14.5" />
    </svg>
  );
}

export function PercivalEmblem(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M6.5 9.5C6.5 6.2 8.9 3.8 12 3.8C15.1 3.8 17.5 6.2 17.5 9.5V16.2L12 20.2L6.5 16.2V9.5Z" />
      <path d="M6.5 11.2H17.5" />
      <path d="M12 7.8V18.8" />
      <path d="M3.5 7.5L6.5 9.8" />
      <path d="M20.5 7.5L17.5 9.8" />
    </svg>
  );
}

export function ServantEmblem(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M12 3L5.2 5.8V11.8C5.2 16.3 8.1 19.7 12 21.2C15.9 19.7 18.8 16.3 18.8 11.8V5.8L12 3Z" />
      <path d="M12 7V16.8" />
      <path d="M8.8 11.2H15.2" />
    </svg>
  );
}

export function AssassinEmblem(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M19.5 4.5L9.2 14.8" />
      <path d="M15.2 4.5H19.5V8.8" />
      <path d="M6.8 12.4L11.6 17.2" />
      <path d="M4.5 19.5L9.2 14.8" />
      <circle cx="9.5" cy="9.5" r="5.8" strokeDasharray="2.5 2.5" />
    </svg>
  );
}

export function MorganaEmblem(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M4.5 8.5L7.8 12L12 5.2L16.2 12L19.5 8.5L17.8 17.5H6.2L4.5 8.5Z" />
      <circle cx="12" cy="13.8" r="1.6" />
      <path d="M6.5 20.2H17.5" />
    </svg>
  );
}

export function MordredEmblem(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M4.5 4.2L7.5 9.2V16.5L12 20.5L16.5 16.5V9.2L19.5 4.2L14.5 7.5L12 5.5L9.5 7.5L4.5 4.2Z" />
      <path d="M9.2 12.5L11 13.6" />
      <path d="M14.8 12.5L13 13.6" />
      <path d="M12 13.8V18.5" />
    </svg>
  );
}

export function OberonEmblem(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M6.5 15.2V10.5C6.5 7.2 8.9 4.5 12 4.5C15.1 4.5 17.5 7.2 17.5 10.5V15.2" />
      <circle cx="9.8" cy="10.8" r="1" fill="currentColor" />
      <circle cx="14.2" cy="10.8" r="1" fill="currentColor" />
      <path d="M4 17.8C6.8 16.4 9.2 19.2 12 17.8C14.8 16.4 17.2 19.2 20 17.8" />
      <path d="M6 20.5C8.5 19.5 10.2 21.2 12 20.5C13.8 19.8 15.5 21.2 18 20.5" />
    </svg>
  );
}

export function MinionEmblem(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M12 3.5L18.5 7V13C18.5 16.8 15.6 19.8 12 21C8.4 19.8 5.5 16.8 5.5 13V7L12 3.5Z" />
      <path d="M8.8 9.2L15.2 15.6" />
      <path d="M15.2 9.2L8.8 15.6" />
    </svg>
  );
}

export function ExcaliburSealEmblem(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <circle cx="12" cy="12" r="8.8" strokeDasharray="3 2" />
      <path d="M12 3.2V16.5" />
      <path d="M8.5 7.2H15.5" />
      <path d="M7.2 18.8L9.5 15.8H14.5L16.8 18.8H7.2Z" />
    </svg>
  );
}

export function RoleEmblem({ role, className }: { role?: Role; className?: string }) {
  switch (role) {
    case "MERLIN":
      return <MerlinEmblem className={className} />;
    case "PERCIVAL":
      return <PercivalEmblem className={className} />;
    case "SERVANT":
      return <ServantEmblem className={className} />;
    case "ASSASSIN":
      return <AssassinEmblem className={className} />;
    case "MORGANA":
      return <MorganaEmblem className={className} />;
    case "MORDRED":
      return <MordredEmblem className={className} />;
    case "OBERON":
      return <OberonEmblem className={className} />;
    case "MINION":
      return <MinionEmblem className={className} />;
    default:
      return <ExcaliburSealEmblem className={className} />;
  }
}

export function HolyGrailIcon(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M7 4.5H17V9.2C17 12 14.8 14.2 12 14.2C9.2 14.2 7 12 7 9.2V4.5Z" />
      <path d="M7 6.5H4.8C4.8 9.2 5.8 10.8 7.5 11.2" />
      <path d="M17 6.5H19.2C19.2 9.2 18.2 10.8 16.5 11.2" />
      <path d="M12 14.2V19.5" />
      <path d="M8.2 19.5H15.8" />
    </svg>
  );
}

export function SabotageDaggerIcon(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.85"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M18.8 5.2L8.5 15.5" />
      <path d="M14.5 5.2H18.8V9.5" />
      <path d="M6.2 13.2L10.8 17.8" />
      <path d="M5.2 18.8L8.5 15.5" />
    </svg>
  );
}

export function CrownLeaderIcon(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.9"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M3.8 18.5H20.2" />
      <path d="M4.8 15.5L3.5 7.2L8.5 10.8L12 5.2L15.5 10.8L20.5 7.2L19.2 15.5H4.8Z" />
    </svg>
  );
}

export function ExpeditionShieldIcon(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.85"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M12 3.2L5.5 6V11.6C5.5 15.8 8.3 19.2 12 20.8C15.7 19.2 18.5 15.8 18.5 11.6V6L12 3.2Z" />
      <path d="M9.2 12L11.2 14L15.2 9.8" />
    </svg>
  );
}

export function WhiteVoteSealIcon(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.85"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <circle cx="12" cy="12" r="8.5" />
      <circle cx="12" cy="12" r="5.8" strokeDasharray="2 2" />
      <path d="M8.8 12.2L11 14.4L15.4 9.8" />
    </svg>
  );
}

export function BlackVoteSealIcon(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.85"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <circle cx="12" cy="12" r="8.5" />
      <path d="M9.2 9.2L14.8 14.8" />
      <path d="M14.8 9.2L9.2 14.8" />
    </svg>
  );
}

export function EyeVeilIcon({ hidden, className }: { hidden: boolean; className?: string }) {
  if (hidden) {
    return (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        className={className}
      >
        <path d="M3 3l18 18" />
        <path d="M10.6 10.6a2 2 0 0 0 2.8 2.8" />
        <path d="M9.9 5.1A10.4 10.4 0 0 1 12 4.9c5 0 8.8 3.6 10 7.1a11.7 11.7 0 0 1-4 5.2" />
        <path d="M6.2 6.2A11.5 11.5 0 0 0 2 12c1.2 3.5 5 7.1 10 7.1 1.8 0 3.5-.5 5-1.3" />
      </svg>
    );
  }
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
    >
      <path d="M2 12s3.8-7 10-7 10 7 10 7-3.8 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="2.8" />
    </svg>
  );
}

export function ChevronIcon({ open, className }: { open: boolean; className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      style={{
        transform: open ? "rotate(180deg)" : "rotate(0deg)",
        transition: "transform 0.2s ease",
      }}
    >
      <path d="m18 15-6-6-6 6" />
    </svg>
  );
}

export function ScrollLedgerIcon(props: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M6.5 4.5H17.5C18.6 4.5 19.5 5.4 19.5 6.5V18.5C19.5 19.6 18.6 20.5 17.5 20.5H6.5C5.4 20.5 4.5 19.6 4.5 18.5V6.5C4.5 5.4 5.4 4.5 6.5 4.5Z" />
      <path d="M8.5 9H15.5" />
      <path d="M8.5 13H15.5" />
      <path d="M8.5 17H12.5" />
    </svg>
  );
}

export function ThemeSunMoonIcon({ isLight, className }: { isLight: boolean; className?: string }) {
  if (isLight) {
    return (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        className={className}
      >
        <circle cx="12" cy="12" r="4.2" />
        <path d="M12 2.5v2.2M12 19.3v2.2M4.9 4.9l1.6 1.6M17.5 17.5l1.6 1.6M2.5 12h2.2M19.3 12h2.2M4.9 19.1l1.6-1.6M17.5 6.5l1.6-1.6" />
      </svg>
    );
  }
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
    >
      <path d="M19.5 14.5A7.5 7.5 0 0 1 9.5 4.5a7.5 7.5 0 1 0 10 10Z" />
    </svg>
  );
}

export function RoundTableRunesSvg(props: IconProps) {
  return (
    <svg viewBox="0 0 200 200" fill="none" stroke="currentColor" {...props}>
      <circle cx="100" cy="100" r="94" strokeWidth="1.2" strokeOpacity="0.38" />
      <circle
        cx="100"
        cy="100"
        r="86"
        strokeWidth="1.4"
        strokeDasharray="4 5"
        strokeOpacity="0.55"
      />
      <circle cx="100" cy="100" r="68" strokeWidth="1" strokeOpacity="0.28" />
      <path
        d="M100 10V26M100 174V190M10 100H26M174 100H190M36.4 36.4L47.7 47.7M152.3 152.3L163.6 163.6M163.6 36.4L152.3 47.7M47.7 152.3L36.4 163.6"
        strokeWidth="1.2"
        strokeOpacity="0.35"
        strokeLinecap="round"
      />
      <polygon
        points="100,24 106,40 122,40 109,50 114,66 100,56 86,66 91,50 78,40 94,40"
        strokeWidth="0.9"
        strokeOpacity="0.22"
      />
    </svg>
  );
}

