import { FAQ } from "../faq";

export function Faq() {
  return (
    <section className="faq">
      <h2>FAQ</h2>
      {FAQ.map((section) => (
        <div key={section.title} className="faq-section">
          <h3>{section.title}</h3>
          {section.entries.map((entry) => (
            <details key={entry.question}>
              <summary>{entry.question}</summary>
              {entry.answer.map((paragraph) => (
                <p key={paragraph}>{paragraph}</p>
              ))}
            </details>
          ))}
        </div>
      ))}
    </section>
  );
}
