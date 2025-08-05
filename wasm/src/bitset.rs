#[cfg(test)]
pub mod tests {

    pub fn print_bitset(bitset: u64) {
        let mut buffer = String::new();
        print_bitset_to(bitset, &mut buffer).unwrap();
        println!("{buffer}");
    }

    fn print_bitset_to<W: std::fmt::Write>(bitset: u64, writer: &mut W) -> std::fmt::Result {
        let mut lines = vec![];
        lines.push("+-A-B-C-D-E-F-G-H-+".to_string());
        for row in 0..8 {
            let mut line = String::new();
            line.push_str(&format!("{} ", row + 1));
            for col in 0..8 {
                let index = row * 8 + col;
                let mask = 1u64 << index;
                if bitset & mask != 0 {
                    line.push_str("● ");
                } else {
                    line.push_str("  ");
                }
            }
            line.push_str(&format!("{}", row + 1));
            lines.push(line);
        }
        lines.push("+-A-B-C-D-E-F-G-H-+".to_string());

        write!(writer, "{}", lines.join("\n") + "\n")
    }
}
