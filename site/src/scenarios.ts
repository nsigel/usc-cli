// What the hero animation plays. Each scenario is one question asked two ways:
// clicking through the site, and asking an agent that runs usc.
export interface Scenario {
  question: string;
  site: { name: string; host: string; tabs: string[]; cards: string[]; target: number };
  calls: string[];
  answer: string;
}

export const scenarios: Scenario[] = [
  {
    question: "when's that econ test btw",
    site: {
      name: "Brightspace",
      host: "brightspace.usc.edu",
      tabs: ["Home", "Content", "Grades"],
      cards: ["ECON 203", "CSCI 270", "WRIT 150", "BUAD 306"],
      target: 0,
    },
    calls: ["usc brightspace courses", "usc brightspace announcements 148230"],
    answer:
      "ECON 203 Midterm 2 is Thursday, Nov 20, 2:00–3:20 pm in SLH 100. It covers chapters 9–12 and you can bring one page of notes.",
  },
  {
    question: "any leavey rooms for 5 tomorrow around 3?",
    site: {
      name: "LibCal",
      host: "libcal.usc.edu",
      tabs: ["Spaces", "Hours", "Policies"],
      cards: ["Level 1", "Level 2", "Level 3", "Pods"],
      target: 2,
    },
    calls: ["usc libcal schedule --date tomorrow --category rooms --min-capacity 5"],
    answer: "Room 304 seats 6 and is free 3:00–5:00 pm tomorrow. Want me to book 3:00–4:30?",
  },
  {
    question: "is BUAD 306 still open for spring",
    site: {
      name: "WebReg",
      host: "webreg.usc.edu",
      tabs: ["Terms", "Course Bin", "Calendar"],
      cards: ["Spring 2027", "Summer 2027", "Fall 2026", "Help"],
      target: 0,
    },
    calls: ["usc webreg terms", "usc webreg courses --term 20271 --course BUAD-306"],
    answer:
      "Two lecture sections have seats: 14790 (MW 10:00, 3 left) and 14795 (TTh 2:00, 11 left). Add one to your course bin?",
  },
  {
    question: "consulting career fairs this month?",
    site: {
      name: "Handshake",
      host: "usc.joinhandshake.com",
      tabs: ["Jobs", "Events", "Fairs"],
      cards: ["Career fairs", "Workshops", "Info sessions", "Saved"],
      target: 0,
    },
    calls: ["usc handshake career-fairs --keyword consulting --date next-30"],
    answer: "One: the Marshall Consulting & Strategy Fair, Nov 19, 11 am–3 pm at Town & Gown. 42 employers so far.",
  },
  {
    question: "did my 270 hw grade post",
    site: {
      name: "Brightspace",
      host: "brightspace.usc.edu",
      tabs: ["Home", "Content", "Grades"],
      cards: ["ECON 203", "CSCI 270", "WRIT 150", "BUAD 306"],
      target: 1,
    },
    calls: ["usc brightspace grades 150112 --graded-only"],
    answer: "Yes, Homework 6 posted this morning: 46/50. Your course total is 91.2%.",
  },
];
